// Copyright © BaseHarbor and contributors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package machinehttp
package machinehttp

// store_retention.go
// Retention and compaction helpers for the execution journal. Extracted from
// store.go so that store.go stays within the 800-line readability limit while
// every helper keeps its original behaviour and call site. The active-segment
// and numbered-backup rotation is newest-per-id compacted so that the newest
// line per execution-id always survives rotation (review-1354e finding 1):
// superseded occurrences for an id are dropped, active/unknown states are never
// dropped. These helpers are package-level and used by the rotate/write paths in
// store.go; they hold no assumptions beyond what executionStore already guards
// (callers must hold s.mu).

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// keepNewestAcross returns the lines of inSeg with any line whose id already
// appeared in already-seen (newer) dropped. It keeps the newest occurrence per
// id and drops older superseded occurrences.
func keepNewestAcross(inSeg, seenNewest []byte) []byte {
	lines := splitLines(inSeg)
	lastIndex := map[string]int{}
	lineIds := make([]string, len(lines))
	for i, raw := range lines {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		id := lineExecutionID(raw)
		lineIds[i] = id
		lastIndex[id] = i
	}
	// Mark ids already present in newer segments as already-seen so their older
	// occurrences here are dropped.
	seen := map[string]bool{}
	for _, raw := range splitLines(seenNewest) {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		seen[lineExecutionID(raw)] = true
	}
	out := make([]byte, 0, len(inSeg))
	for i, raw := range lines {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		id := lineIds[i]
		// Keep this line only if it is the newest occurrence for its id both within
		// this segment AND not shadowed by a newer occurrence in an older segment.
		if lastIndex[id] == i && !seen[id] {
			out = append(out, raw...)
			out = append(out, '\n')
		}
	}
	return out
}

// compactLines returns the bytes of an append-only journal with all but the last
// line per execution-id removed. The last (newest) line per id is always kept.
func compactLines(data []byte) []byte {
	lines := splitLines(data)
	lastIndex := map[string]int{}
	lineIds := make([]string, len(lines))
	for i, raw := range lines {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		id := lineExecutionID(raw)
		lineIds[i] = id
		lastIndex[id] = i
	}
	kept := make([]byte, 0, len(data))
	for i, raw := range lines {
		if lastIndex[lineIds[i]] == i {
			kept = append(kept, raw...)
			kept = append(kept, '\n')
		}
	}
	return kept
}

// splitIntoCappedSegments splits a flattened journal byte slice into segments,
// each at most journalSegmentCap bytes (plus the trailing newline each line
// carries). The active segment is the first element; numbered backups follow.
func splitIntoCappedSegments(data []byte) [][]byte {
	var segments [][]byte
	var cur []byte
	for i := 0; i < len(data); {
		// Each line ends at the next '\n'; include it.
		j := i
		for j < len(data) && data[j] != '\n' {
			j++
		}
		lineLen := j - i + 1 // +1 for the newline
		if len(cur) > 0 && len(cur)+lineLen > int(journalSegmentCap) {
			segments = append(segments, cur)
			cur = nil
		}
		cur = append(cur, data[i:j+1]...)
		i = j + 1
	}
	if len(cur) > 0 {
		segments = append(segments, cur)
	}
	return segments
}

// segmentTargets returns the on-disk path for each compacted segment in order:
// the active segment first (journal.jsonl), then numbered backups .1..N.
func (s *executionStore) segmentTargets(count int) []string {
	targets := make([]string, 0, count)
	targets = append(targets, s.journalPath)
	for i := 1; i < count; i++ {
		targets = append(targets, s.backupPath(i))
	}
	return targets
}

// lineExecutionID extracts the execution_id of a single JSON journal line.
func lineExecutionID(raw string) string {
	var line struct {
		ExecutionID string `json:"execution_id"`
	}
	if err := json.Unmarshal([]byte(raw), &line); err != nil {
		return ""
	}
	return line.ExecutionID
}

// enforceTotalCapLocked rotates until total journal bytes never exceed
// journalSegmentTotalCap. Each iteration calls rotateSegmentLocked, which
// compacts globally first (so the newest line per execution-id is always retained
// across every backup — a plain file shift would bury the newest line per id under
// superseded occurrences in older backups, review-1354e) and then rotates the
// active segment into .1 while dropping the overflow backup. Each rotation removes
// one backup's worth of oldest data, so the loop terminates in finitely many
// steps. Because every rotation is newest-per-id compacted, no execution-id loses
// its newest line while the total cap holds.
func (s *executionStore) enforceTotalCapLocked() error {
	for s.totalJournalBytes() > journalSegmentTotalCap {
		if err := s.rotateSegmentLocked(); err != nil {
			return err
		}
	}
	return nil
}

// writeCompactedJournal writes a globally-compacted, newest-per-id journal back
// to disk in a single atomically-swapped, segment-capped set of files.
func (s *executionStore) writeCompactedJournal(newest []byte) error {
	if len(newest) == 0 {
		return nil
	}
	segments := splitIntoCappedSegments(newest)
	var tmpNames []string
	for i, seg := range segments {
		tmp := fmt.Sprintf("%s.compact.%d", s.journalPath, i)
		if err := os.WriteFile(tmp, seg, 0o600); err != nil {
			for _, n := range tmpNames {
				_ = os.Remove(n)
			}
			return fmt.Errorf("write compacted segment %d: %w", i, err)
		}
		tmpNames = append(tmpNames, tmp)
	}
	targets := s.segmentTargets(len(segments))
	for i, tmp := range tmpNames {
		if err := os.Rename(tmp, targets[i]); err != nil {
			return fmt.Errorf("replace compacted segment %d: %w", i, err)
		}
	}
	return nil
}

// trimOldestIDsFromKeptLines returns the kept bytes with whole oldest
// execution-ids dropped from the end (oldest-first) until the byte budget is met
// or an active/unknown id is reached. The second return value reports whether
// any active/unknown id remains in the keep-list. The newest-state for each id is
// read from the in-memory newest-per-id bytes (already parsed) so trimming is
// linear in the journal size rather than O(n) disk re-reads per id.
func (s *executionStore) trimOldestIDsFromKeptLines(newest []byte) ([]byte, bool) {
	lines := splitLines(newest)
	lineIDs := make([]string, len(lines))
	lineStates := make([]machine.ExecutionState, len(lines))
	for i, raw := range lines {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var line struct {
			ExecutionID string                 `json:"execution_id"`
			State       machine.ExecutionState `json:"state"`
		}
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			continue
		}
		lineIDs[i] = line.ExecutionID
		lineStates[i] = line.State
	}
	// Distinct ids in newest-first occurrence order (first appearance scanning
	// newest-to-oldest).
	var order []string
	seen := map[string]bool{}
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		id := lineIDs[i]
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		order = append(order, id)
	}
	// Walk oldest-first (reverse of order) and drop whole ids whose newest state
	// is terminal/superseded, stopping at the first active/unknown id.
	var dropped map[string]bool
	remainingBytes := s.totalJournalBytes()
	for i := len(order) - 1; i >= 0; i-- {
		id := order[i]
		// Its newest line is the FIRST occurrence scanning newest-first.
		var state machine.ExecutionState
		found := false
		for j := len(lines) - 1; j >= 0; j-- {
			if lineIDs[j] == id {
				state = lineStates[j]
				found = true
				break
			}
		}
		if !found || !isTerminalState(state) {
			break
		}
		if dropped == nil {
			dropped = map[string]bool{}
		}
		dropped[id] = true
		for j := len(lines) - 1; j >= 0; j-- {
			if lineIDs[j] == id {
				remainingBytes -= int64(len(lines[j]) + 1)
				break
			}
		}
		if remainingBytes <= journalSegmentTotalCap {
			break
		}
	}
	if len(dropped) == 0 {
		return newest, false
	}
	out := make([]byte, 0, len(newest))
	for i, raw := range lines {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if dropped[lineIDs[i]] {
			continue
		}
		out = append(out, raw...)
		out = append(out, '\n')
	}
	return out, true
}

// isTerminalState reports whether an execution state is terminal
// (succeeded/failed/cancelled), which is safe to drop by retention. A
// pending/running/unknown state is active and never dropped.
func isTerminalState(state machine.ExecutionState) bool {
	switch state {
	case machine.ExecutionSucceeded,
		machine.ExecutionFailed,
		machine.ExecutionCancelled:
		return true
	default:
		return false
	}
}

// newestStateForID is retained for callers that need an id's state out of the
// on-disk journal directly. It reads segment-by-segment newest-first (active
// first, then .1..N) and returns the newest line's state for id.
func (s *executionStore) newestStateForID(id string) (machine.ExecutionState, bool) {
	segments, err := s.journalSegments()
	if err != nil {
		return "", false
	}
	for _, seg := range segments {
		lines := splitLines(seg)
		for i := len(lines) - 1; i >= 0; i-- {
			if strings.TrimSpace(lines[i]) == "" {
				continue
			}
			var line struct {
				ExecutionID string                 `json:"execution_id"`
				State       machine.ExecutionState `json:"state"`
			}
			if err := json.Unmarshal([]byte(lines[i]), &line); err != nil {
				continue
			}
			if line.ExecutionID == id {
				return line.State, true
			}
		}
	}
	return "", false
}
