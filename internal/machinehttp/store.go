package machinehttp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

const maxExecutionEvents = 512

// currentJournalDir returns the directory that holds the active journal segment
// and its numbered backups. It is package-level so retention accounting can sum
// total bytes even before a store instance is bound to a path.
func (s *executionStore) currentJournalDir() string {
	return s.journalDir
}

// Execution-journal retention (Owner scope decision 2026-11-03, card
// t-20261022-r27-ci-authority round 3). The card t-20261018-r27-execution-journal
// line 42 requires a *bound* retention without naming a number, so the bound is a
// local engineering constant here: each journal segment is capped and rotated to
// numbered backups; the four segments bound total disk use; and rotation compacted
// so the newest line per execution-id is always retained.
const (
	// journalSegmentCap bounds a single journal segment. Crossing it rotates the
	// active segment to the next numbered backup.
	journalSegmentCap int64 = 4 << 20 // 4 MiB
	// journalBackupMax keeps at most this many numbered backups (journal.jsonl.1..N).
	journalBackupMax = 3
	// journalSegmentTotalCap is the hard upper bound on total journal bytes across
	// the active segment plus all backups.
	journalSegmentTotalCap int64 = 16 << 20 // 16 MiB
)

// journalDir returns the directory that persists execution-journal records under
// a state-root. An empty state-root leaves the store purely in-memory (existing
// behavior); the production caller supplies the runtime state-root so the
// journal survives process restarts.
func journalDir(stateRoot string) string {
	if stateRoot == "" {
		return ""
	}
	return filepath.Join(stateRoot, "journal")
}

type executionRecord struct {
	execution   machine.Execution
	events      []machine.MachineEvent
	nextSeq     uint64
	subscribers map[chan machine.MachineEvent]struct{}
}

type executionStore struct {
	mu          sync.RWMutex
	records     map[string]*executionRecord
	journalPath string
	journalDir  string
}

func newExecutionStore() *executionStore {
	return &executionStore{records: map[string]*executionRecord{}}
}

// newPersistedExecutionStore loads any existing journal from stateRoot and
// persists every subsequent mutation to it. stateRoot == "" keeps the store
// in-memory. A corrupt journal line fails closed (never silently skipped).
func newPersistedExecutionStore(stateRoot string) (*executionStore, error) {
	store := &executionStore{records: map[string]*executionRecord{}}
	dir := journalDir(stateRoot)
	if dir == "" {
		return store, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create execution journal dir %q: %w", dir, err)
	}
	store.journalPath = filepath.Join(dir, "journal.jsonl")
	store.journalDir = dir
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func newExecutionID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "exec_" + hex.EncodeToString(raw[:]), nil
}

func (s *executionStore) create(operationID string, actor machine.ActorRef, ctx machine.OperationContext) (machine.Execution, error) {
	id, err := newExecutionID()
	if err != nil {
		return machine.Execution{}, err
	}
	execution := machine.Execution{
		ContractVersion: machine.ExecutionContractVersion,
		ExecutionID:     id,
		OperationID:     operationID,
		Actor:           actor,
		Context:         ctx,
		State:           machine.ExecutionPending,
	}
	// Persist the accepted commitment before returning 202 so an accepted
	// execution survives a crash between acceptance and the first transition
	// (review finding 1). The line carries only identity-critical fields:
	// execution id, state, actor, context and timestamps — never result/
	// progress/error payloads.
	s.mu.Lock()
	record := &executionRecord{
		execution:   execution,
		subscribers: map[chan machine.MachineEvent]struct{}{},
	}
	s.records[id] = record
	if err := s.appendJournalLocked(record); err != nil {
		delete(s.records, id)
		s.mu.Unlock()
		return machine.Execution{}, err
	}
	s.mu.Unlock()
	return execution, nil
}

func (s *executionStore) get(id string) (machine.Execution, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[id]
	if !ok {
		return machine.Execution{}, false
	}
	return record.execution, true
}

func (s *executionStore) start(id string) error {
	now := time.Now().UTC()
	return s.update(id,
		func(record *executionRecord) {
			record.execution.State = machine.ExecutionRunning
			record.execution.StartedAt = &now
		},
		func(record *executionRecord) {
			s.emitLocked(record, machine.MachineEvent{
				Kind:        machine.EventOperationStarted,
				ExecutionID: id,
				OperationID: record.execution.OperationID,
				Actor:       &record.execution.Actor,
				Context:     &record.execution.Context,
				State:       machine.ExecutionRunning,
			})
		})
}

func (s *executionStore) progress(id string, progress machine.OperationProgress) error {
	copy := progress
	return s.update(id,
		func(record *executionRecord) {
			record.execution.Progress = &copy
		},
		func(record *executionRecord) {
			s.emitLocked(record, machine.MachineEvent{
				Kind:        machine.EventOperationProgress,
				ExecutionID: id,
				OperationID: record.execution.OperationID,
				State:       record.execution.State,
				Progress:    &copy,
			})
		})
}

func (s *executionStore) succeed(id string, result []byte) error {
	now := time.Now().UTC()
	resultCopy := append([]byte(nil), result...)
	return s.update(id,
		func(record *executionRecord) {
			record.execution.State = machine.ExecutionSucceeded
			record.execution.FinishedAt = &now
			record.execution.Result = resultCopy
			record.execution.Error = nil
		},
		func(record *executionRecord) {
			s.emitLocked(record, machine.MachineEvent{
				Kind:        machine.EventOperationSucceeded,
				ExecutionID: id,
				OperationID: record.execution.OperationID,
				State:       machine.ExecutionSucceeded,
				Result:      resultCopy,
			})
		})
}

func (s *executionStore) fail(id string, err error) error {
	now := time.Now().UTC()
	classified := machine.Classify(err)
	return s.update(id,
		func(record *executionRecord) {
			record.execution.State = machine.ExecutionFailed
			record.execution.FinishedAt = &now
			record.execution.Error = classified
		},
		func(record *executionRecord) {
			s.emitLocked(record, machine.MachineEvent{
				Kind:        machine.EventOperationFailed,
				ExecutionID: id,
				OperationID: record.execution.OperationID,
				State:       machine.ExecutionFailed,
				Error:       classified,
			})
		})
}

// update applies a state mutation and emits the resulting event under a strict
// persist-before-publish order (review finding 2, round 4). mutate(record) sets
// the new in-memory state; onCommit(record) publishes on subscriber channels.
// The mutation is applied first so appendJournalLocked writes the NEW state, but
// onCommit is only invoked AFTER the append has succeeded: a failed append rolls
// the record back to prior state and returns the error without ever publishing an
// event for a state that was never durably committed (fail closed).
func (s *executionStore) update(id string, mutate func(*executionRecord), onCommit func(*executionRecord)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return errors.New("execution not found")
	}
	prior := *record
	mutate(record)
	if err := s.appendJournalLocked(record); err != nil {
		*record = prior
		return err
	}
	onCommit(record)
	return nil
}

// appendJournalLocked writes the record's current state as one append-only
// journal line. Callers must hold s.mu. A persistence error is surfaced to the
// caller (fail closed on write) rather than swallowed. Only identity- and
// status-critical fields are written: execution id, state and timestamps. The
// authorized request commitment (actor/context) is written when it first
// becomes available (create) so it survives a restart, and every subsequent line
// re-emits it so rotation compaction keeps a complete newest line per id.
// Result, progress and error payloads are deliberately NOT written: they carry
// artifact/secret material and are reconstructable from the execution record.
func (s *executionStore) appendJournalLocked(record *executionRecord) error {
	if s.journalPath == "" {
		return nil
	}
	// Hard per-entry limit (review finding 3, mission runde 4): marshal first so
	// we can measure the real line length, then reject with an error any single
	// entry that would exceed the segment cap before it is ever appended. This is
	// what keeps a 5 MiB actor from producing a > 4 MiB journal segment.
	line, err := json.Marshal(struct {
		ExecutionID string                   `json:"execution_id"`
		OperationID string                   `json:"operation_id"`
		State       machine.ExecutionState   `json:"state"`
		Actor       machine.ActorRef         `json:"actor,omitempty"`
		Context     machine.OperationContext `json:"context,omitempty"`
		StartedAt   *time.Time               `json:"started_at,omitempty"`
		FinishedAt  *time.Time               `json:"finished_at,omitempty"`
	}{
		ExecutionID: record.execution.ExecutionID,
		OperationID: record.execution.OperationID,
		State:       record.execution.State,
		Actor:       record.execution.Actor,
		Context:     record.execution.Context,
		StartedAt:   record.execution.StartedAt,
		FinishedAt:  record.execution.FinishedAt,
	})
	if err != nil {
		return fmt.Errorf("marshal execution journal line: %w", err)
	}
	if int64(len(line)) > journalSegmentCap {
		return fmt.Errorf("execution journal entry %d bytes exceeds hard segment cap %d: reject oversized entry (finding 3)", len(line), journalSegmentCap)
	}
	if err := s.appendJournalLineLocked(line); err != nil {
		return err
	}
	// Bounded retention: rotate the active segment past its cap and compact so the
	// newest line per execution-id is always retained.
	if err := s.rotateIfNeededLocked(); err != nil {
		return err
	}
	return nil
}

// appendJournalLineLocked appends one already-marshaled line to the active
// segment and enforces the hard total-cap after the append. Callers must hold
// s.mu.
//
// Segment-cap is enforced PRE-WRITE (review-1354d finding 3, mission runde 6):
// the check must decide the size AFTER appending, not before. If the current
// segment already holds data and adding this line would exceed
// journalSegmentCap, the active segment is rotated atomically BEFORE the new
// line is appended, so two individually-allowed entries can never produce a
// single > 4 MiB segment (e.g. two 430 KiB NUL-actor lines). A read-only or
// otherwise unwritable active segment — where no rotation is possible — fails
// closed: the line is appended and the append error propagates so NO event is
// ever published for a write that could not be durably committed.
//
// Recipe B3 (review-1354g): durability is committed as write→Sync→Close, and a
// failure at any of those steps returns an error before commit so `update`
// rolls the in-memory state back and publishes no event.
func (s *executionStore) appendJournalLineLocked(line []byte) error {
	if err := s.rotateBeforeWriteLocked(len(line) + 1); err != nil {
		return err
	}
	f, err := os.OpenFile(s.journalPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open execution journal %q: %w", s.journalPath, err)
	}
	// Recipe B3 commit path: write, fsync, close — each error fails closed. The
	// explicit Close (not `defer`) means a Close-time flush failure is surfaced
	// here, before the transition is committed and before any event is published;
	// `update` rolls the in-memory state back and never emits for a write that
	// could not be durably committed.
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return fmt.Errorf("append execution journal line: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("fsync execution journal line: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close execution journal line: %w", err)
	}
	return s.enforceTotalCapLocked()
}

// rotateBeforeWriteLocked rotates the active segment atomically when appending
// the given line length would exceed journalSegmentCap. The "+1" accounts for
// the trailing newline. When no active segment exists yet the check is a no-op
// (the first line of a fresh segment is always safe: a single entry was
// already rejected above in appendJournalLocked when it would exceed the cap).
// Any rotation error is returned so the caller fails closed — a segment that
// cannot be rotated also cannot accept the line.
func (s *executionStore) rotateBeforeWriteLocked(extraBytes int) error {
	info, err := os.Stat(s.journalPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.Size()+int64(extraBytes) <= journalSegmentCap {
		return nil
	}
	return s.rotateSegmentLocked()
}

// rotateIfNeededLocked rotates the active segment into a numbered backup when it
// exceeds journalSegmentCap. It compacts every segment newest-first (global
// compaction) so the newest line per execution-id survives the rotation:
// superseded lines for an id are dropped but the newest occurrence is always
// kept, and active/unknown states are never dropped. Callers must hold s.mu.
func (s *executionStore) rotateIfNeededLocked() error {
	info, err := os.Stat(s.journalPath)
	if err != nil {
		return nil
	}
	if info.Size() <= journalSegmentCap {
		return nil
	}
	if err := s.compactAllSegmentsLocked(); err != nil {
		return err
	}
	if err := s.rotateSegmentLocked(); err != nil {
		return err
	}
	return nil
}

// compactAllSegmentsLocked performs a global compaction: it reads every journal
// segment newest-first (active first, then .1..N), keeps only the newest line per
// execution-id across all of them, and swaps the freshly written, segment-capped
// segments in atomically. This is what makes rotation lossless — a rotation only
// shifts files, but without this compaction the newest line per id can be buried
// under superseded occurrences in an older backup that the active-segment-only
// compaction never touched (review-1354e). Active/unknown states are never
// dropped; only superseded older lines for an id are removed. It is a no-op when
// every line across every segment is already unique.
//
// Each compacted segment is capped at journalSegmentCap before being written so
// the swap cannot itself produce a > cap segment (e.g. two large active ids).
// Callers must hold s.mu.
func (s *executionStore) compactAllSegmentsLocked() error {
	segments, err := s.journalSegments()
	if err != nil {
		return err
	}
	// Newest-first order: the active segment (returned first by journalSegments)
	// wins over numbered backups.
	var newest []byte
	for _, seg := range segments {
		kept := compactLines(seg)
		// The newest line per id in this segment supersedes any occurrence in
		// older segments already absorbed, so drop older duplicates across the
		// whole running set.
		kept = keepNewestAcross(kept, newest)
		newest = append(newest, kept...)
	}
	if len(newest) == 0 {
		return nil
	}
	return s.writeCompactedJournal(newest)
}

// rotateSegmentLocked rolls the active segment into journal.jsonl.1 (shifting
// existing backups up). It compacts globally first so the newest line per
// execution-id survives the shift — a plain file shift alone would bury the
// newest line per id under superseded occurrences in older backups that a
// naive rotate-remove-oldest could not reach. It is a no-op when no active
// segment exists.
func (s *executionStore) rotateSegmentLocked() error {
	if !exists(s.journalPath) {
		return nil
	}
	if err := s.compactAllSegmentsLocked(); err != nil {
		return err
	}
	// Shift backups up: .N -> .N+1 ... .1 -> .2. This may momentarily create an
	// overflow at .journalBackupMax+1, which is removed below so the active segment
	// plus at most journalBackupMax numbered backups is never exceeded.
	for i := journalBackupMax; i >= 1; i-- {
		src := s.backupPath(i)
		dst := s.backupPath(i + 1)
		if !exists(src) {
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			return err
		}
	}
	// Drop the overflow backup created by the shift above.
	if err := os.Remove(s.backupPath(journalBackupMax + 1)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(s.journalPath, s.backupPath(1)); err != nil {
		return fmt.Errorf("rotate execution journal: %w", err)
	}
	return nil
}

// backupPath returns the path of a numbered backup within the journal directory.
func (s *executionStore) backupPath(n int) string {
	return filepath.Join(filepath.Dir(s.journalPath), fmt.Sprintf("journal.jsonl.%d", n))
}

// exists reports whether a file exists at path.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// totalJournalBytes sums the sizes of the active segment and every backup.
func (s *executionStore) totalJournalBytes() int64 {
	dir := s.journalDir
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var total int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		total += info.Size()
	}
	return total
}

// load reconstructs in-memory records from the journal. It reads the active
// segment and every numbered backup, keeping the newest line per execution-id
// across all of them (append-only ordering is preserved by segment order then
// line order within a segment). It fails closed on a corrupt line: the first
// unparseable record is an error, never a silent skip.
func (s *executionStore) load() error {
	segments, err := s.journalSegments()
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	// Iterate oldest segment first so the newest line per id (in the active
	// segment) wins the overwrite below. journalSegments() returns the active
	// segment first (newest), then numbered backups newest (.1) to oldest (.3);
	// reversing means an older backup cannot overwrite a newer active line — a
	// previously latent ordering bug that surfaces once running states are restored
	// on load (mission runde 4 / review finding 1).
	for _, seg := range reverseSegments(segments) {
		lines := splitLines(seg)
		for i, raw := range lines {
			if strings.TrimSpace(raw) == "" {
				continue
			}
			// Luna recipe B2 (review-1354g): a trailing fragment without its
			// terminating newline was never committed as a complete JSONL record —
			// an interrupted append, even one whose bytes are individually valid
			// JSON. Classify it as corrupt/uncertain and fail closed on load rather
			// than silently restoring it as pending. Only a newline-terminated
			// line is a durable record.
			if i == len(lines)-1 && !bytesHasTrailingNewline(seg) {
				return fmt.Errorf("corrupt execution journal line %d: unterminated final record (interrupted append)", i+1)
			}
			var line struct {
				ExecutionID string                   `json:"execution_id"`
				OperationID string                   `json:"operation_id"`
				State       machine.ExecutionState   `json:"state"`
				Actor       machine.ActorRef         `json:"actor,omitempty"`
				Context     machine.OperationContext `json:"context,omitempty"`
				StartedAt   *time.Time               `json:"started_at,omitempty"`
				FinishedAt  *time.Time               `json:"finished_at,omitempty"`
			}
			if err := json.Unmarshal([]byte(raw), &line); err != nil {
				return fmt.Errorf("corrupt execution journal line %d: %w", i+1, err)
			}
			if strings.TrimSpace(line.ExecutionID) == "" {
				return fmt.Errorf("corrupt execution journal line %d: missing execution_id", i+1)
			}
			// Semantic validation (review finding 2): reject a state that is not one
			// of the real execution states. JSON syntax alone is insufficient — a
			// line like {"execution_id":"x","state":"bogus"} must fail closed.
			if !validExecutionState(line.State) {
				return fmt.Errorf("corrupt execution journal line %d: invalid execution state %q", i+1, line.State)
			}
			// Append-only: later lines are newer, so overwrite — the last valid
			// line per id across all segments wins (both within a segment and across
			// the active-then-backwards segment order). The last persisted state is
			// restored verbatim, including a running execution (mission runde 4 /
			// review finding 1): a Pending→Running line reopened after a crash must
			// come back as running, never downgraded to pending. Semantic validation
			// above already rejects any non-real state, so the value here is trusted.
			if !seen[line.ExecutionID] {
				seen[line.ExecutionID] = true
			}
			record := &executionRecord{
				execution: machine.Execution{
					ContractVersion: machine.ExecutionContractVersion,
					ExecutionID:     line.ExecutionID,
					OperationID:     line.OperationID,
					Actor:           line.Actor,
					Context:         line.Context,
					State:           line.State,
					StartedAt:       line.StartedAt,
					FinishedAt:      line.FinishedAt,
				},
				nextSeq:     0,
				subscribers: map[chan machine.MachineEvent]struct{}{},
			}
			s.records[line.ExecutionID] = record
		}
	}
	return nil
}

// journalSegments returns the journal bytes in load order: the active segment
// first, then numbered backups from newest (.1) to oldest. Callers may assume
// each entry is an append-only JSONL file.
func (s *executionStore) journalSegments() ([][]byte, error) {
	if s.journalDir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(s.journalDir)
	if err != nil {
		return nil, fmt.Errorf("read journal dir %q: %w", s.journalDir, err)
	}
	var names []string
	names = append(names, s.journalPath)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), "journal.jsonl.") {
			names = append(names, filepath.Join(s.journalDir, e.Name()))
		}
	}
	segments := make([][]byte, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read journal segment %q: %w", name, err)
		}
		segments = append(segments, data)
	}
	return segments, nil
}

// validExecutionState reports whether state is one of the real execution states
// or an empty/unknown value. It is the semantic guard used by load: a bogus or
// active state fails closed instead of being resurrected (review finding 2).
func validExecutionState(state machine.ExecutionState) bool {
	switch state {
	case machine.ExecutionPending,
		machine.ExecutionRunning,
		machine.ExecutionSucceeded,
		machine.ExecutionFailed,
		machine.ExecutionCancelled:
		return true
	default:
		return false
	}
}

// reverseSegments returns the journal byte slices from oldest to newest so the
// load loop overwrites older states with newer ones (the newest active segment
// wins). journalSegments() returns newest-first (active, .1, .2, .3).
func reverseSegments(segs [][]byte) [][]byte {
	out := make([][]byte, 0, len(segs))
	for i := len(segs) - 1; i >= 0; i-- {
		out = append(out, segs[i])
	}
	return out
}

// bytesHasTrailingNewline reports whether data ends with a newline byte. An
// append-only journal line is only a durable record once its terminating '\n'
// has been flushed; a final fragment without it is an interrupted write and
// must fail closed on load (Luna recipe B2).
func bytesHasTrailingNewline(data []byte) bool {
	return len(data) > 0 && data[len(data)-1] == '\n'
}

// splitLines splits journal bytes on newlines, tolerating a trailing newline.
func splitLines(data []byte) []string {
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, string(data[start:i]))
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}

func (s *executionStore) emitLocked(record *executionRecord, event machine.MachineEvent) {
	record.nextSeq++
	event.ContractVersion = machine.ExecutionContractVersion
	event.Sequence = record.nextSeq
	event.OccurredAt = time.Now().UTC()
	record.events = append(record.events, event)
	if len(record.events) > maxExecutionEvents {
		record.events = append([]machine.MachineEvent(nil), record.events[len(record.events)-maxExecutionEvents:]...)
	}
	for subscriber := range record.subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
}

func (s *executionStore) subscribe(id string) ([]machine.MachineEvent, <-chan machine.MachineEvent, func(), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return nil, nil, nil, false
	}
	history := append([]machine.MachineEvent(nil), record.events...)
	ch := make(chan machine.MachineEvent, 32)
	record.subscribers[ch] = struct{}{}
	cancel := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if current, exists := s.records[id]; exists {
			if _, subscribed := current.subscribers[ch]; subscribed {
				delete(current.subscribers, ch)
				close(ch)
			}
		}
	}
	return history, ch, cancel, true
}
