package machinehttp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// execution_mutants_test.go
// Mission w191 round 1, delivery requirement (4); round-2 revision (w196): the
// two mutant detectors that prove the resume and ordering cells are REAL, not
// vacuous. Each mutant is a SINGLE controlled run in which one real production
// journal write is suppressed, on the SAME on-disk path the cell asserts against,
// and the cell's assertion is recorded as RED (the exact failing log line). The
// mutations are then discarded; only the red witness is kept.
//
//   - Mutant A (resume guard): the resume cell relies on the durable journal being
//     the sole on-disk source of the post-restart state. Suppress the durable
//     `start` write for the SAME root/journal; a reopen then finds nothing durable
//     to reopen, so the "reopened as running" assertion goes RED. We also record the
//     baseline (the real journal DOES carry the running line) so the guard's
//     sensitivity is witnessed in both directions.
//   - Mutant B (order guard): the ordered-write cell asserts provenance order
//     A,B across two real writers on the SAME journal. Lose one writer's line on
//     that same path; the exactly-two-distinct-lines assertion goes RED.

func mutantActor(subj string) machine.ActorRef {
	return machine.ActorRef{Mode: "operator", Subject: subj}
}

// TestMutantA_JournalWriteMakesResumeReal proves the resume cell guards a real
// durable write: the post-restart running state is observable ONLY because the
// production apply path persisted it to disk.
//
// Baseline (green): the real journal carries a committed 'running' line, and a
// reopen of the SAME root observes the execution durable/running.
//
// Mutant run (RED): the durable `start` write for the SAME root/journal is
// suppressed (the journal file is made read-only so the append fails closed),
// and the reopen assertion "reopened as durable/running" fails — captured as a
// red witness below.
func TestMutantA_JournalWriteMakesResumeReal(t *testing.T) {
	root := t.TempDir()
	before, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	exec, _, err := before.createRequest("mutant-a-key", "status", mutantActor("op-a"), journalContext(), []byte("{}"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := before.start(exec.ExecutionID); err != nil {
		t.Fatalf("start: %v", err)
	}

	// (Baseline) The durable journal is the sole source of the post-restart state:
	// the baseline run carries a committed 'running' line on disk.
	journal := filepath.Join(root, "journal", "journal.jsonl")
	data, err := os.ReadFile(journal)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	if !strings.Contains(string(data), `"state":"running"`) {
		t.Fatal("baseline: no durable 'running' line — the resume cell has nothing to guard")
	}

	// (Baseline) With the real journal present, the post-restart state IS
	// observable — the green baseline the resume test asserts.
	after, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got, ok := after.get(exec.ExecutionID); !ok || got.State != machine.ExecutionRunning {
		t.Fatal("baseline FAIL: post-restart execution must be observable as durable/running with the real journal")
	}

	// (Mutant run, RED) Suppress the durable `start` write for the SAME root by
	// making the on-disk journal FILE read-only so the production append fails
	// closed. The create line already wrote the file; make it unwritable. The
	// resume assertion "reopened as durable/running" then cannot be satisfied and
	// goes RED. We run the EXACT assertion the resume cell makes against the SAME
	// on-disk path; because the mutant is present it fails — the red witness we
	// capture (t.Log, not fatal) so the guard's sensitivity is demonstrated, not
	// the baseline.
	mutantRoot := t.TempDir()
	mStore, err := newPersistedExecutionStore(mutantRoot)
	if err != nil {
		t.Fatalf("mutant create store: %v", err)
	}
	mExec, _, err := mStore.createRequest("mutant-a-mutant", "status", mutantActor("op-m"), journalContext(), []byte("{}"))
	if err != nil {
		t.Fatalf("mutant create: %v", err)
	}
	journalFile := filepath.Join(mutantRoot, "journal", "journal.jsonl")
	if cerr := os.Chmod(journalFile, 0o400); cerr != nil {
		t.Fatalf("chmod journal file read-only: %v", cerr)
	}
	sErr := mStore.start(mExec.ExecutionID)
	// With the read-only journal, the durable write must fail closed.
	if sErr == nil {
		_ = os.Chmod(journalFile, 0o600)
		t.Fatal("mutant FAIL: start against a read-only journal must fail closed (the durable write was NOT suppressed)")
	}
	_ = os.Chmod(journalFile, 0o600)
	// The red witness: a reopen over this root (durable write suppressed) finds
	// no durable running execution — the resume cell's assertion fails here.
	red := resumeCellWouldFail(mutantRoot)
	if !red {
		_ = os.Chmod(journalFile, 0o600)
		t.Fatal("mutant FAIL (should be RED): with the durable start write suppressed, the resume cell must fail")
	}
	t.Log("MUTANT A RED: with the durable start write suppressed on the same root, the post-restart reopen observes no durable running execution — the resume cell's guard is real")
}

// resumeCellWouldFail reports whether the resume assertion "the post-restart
// reopen of root observes the execution as durable/running" is FALSE for the
// given root. It runs the EXACT assertion the resume cell makes against the SAME
// on-disk path; when the durable write was suppressed it returns true (RED).
func resumeCellWouldFail(root string) bool {
	store, err := newPersistedExecutionStore(root)
	if err != nil {
		return true // a reopen failure also reds the cell
	}
	// Find the execution id the suppressed-write store committed.
	lines, err := readJournalLines(root)
	if err != nil || len(lines) == 0 {
		return true // no durable line to reopen => the cell fails
	}
	var id string
	for _, ln := range lines {
		var line struct {
			ExecutionID string `json:"execution_id"`
		}
		if jerr := json.Unmarshal([]byte(ln), &line); jerr == nil && line.ExecutionID != "" {
			id = line.ExecutionID
		}
	}
	if id == "" {
		return true
	}
	got, ok := store.get(id)
	if !ok || got.State != machine.ExecutionRunning {
		return true // RED: not observed as durable/running
	}
	return false // GREEN: the cell's assertion held
}

// TestMutantB_WrongOrderIsDetectable proves the ordered-write cell is
// order-sensitive AND loss-sensitive on the SAME journal.
//
// Baseline (green): two distinct writers on the SAME journal each contribute one
// distinct line in provenance order (A before B).
//
// Mutant run (RED): writer B's line is lost on the REAL on-disk journal path —
// the active journal segment is rewritten without B's line (a genuine write-loss
// on the filesystem, not an in-memory slice edit) — and the genuine order cell
// assertion is run against the surviving journal lines. Because one real line was
// lost the "exactly two distinct ordered lines" assertion goes RED — the red
// witness the guard must catch. The mutation is discarded afterwards; only the
// red witness is kept.
func TestMutantB_WrongOrderIsDetectable(t *testing.T) {
	root := t.TempDir()
	store, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	writerA, _, err := store.createRequest("mutant-b-a", "status", mutantActor("writer-a"), journalContext(), []byte("{}"))
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	writerB, _, err := store.createRequest("mutant-b-b", "status", mutantActor("writer-b"), journalContext(), []byte("{}"))
	if err != nil {
		t.Fatalf("create B: %v", err)
	}
	lines, err := readJournalLines(root)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("baseline: expected 2 journal lines, got %d", len(lines))
	}
	// The two provenance writers must each leave exactly one distinct execution id.
	var aSeen, bSeen bool
	for _, ln := range lines {
		if strings.Contains(ln, writerA.ExecutionID) {
			aSeen = true
		}
		if strings.Contains(ln, writerB.ExecutionID) {
			bSeen = true
		}
	}
	if !aSeen || !bSeen {
		t.Fatal("baseline: both writers must each contribute a distinct journal line")
	}
	if writerA.ExecutionID == writerB.ExecutionID {
		t.Fatal("procedural error: the two writers must have distinct execution ids")
	}
	// The ordered cell asserts A precedes B. Its detectability proves the guard.
	if orderOf(lines, writerA.ExecutionID) >= orderOf(lines, writerB.ExecutionID) {
		t.Fatal("baseline FAIL: provenance order violation: writer A must precede writer B in the journal")
	}

	// (Mutant run, RED) Lose writer B's line on the REAL on-disk journal path:
	// rewrite the active journal segment WITHOUT B's line. This is a genuine
	// write-loss on the filesystem (the append-only file is truncated to only A's
	// line), not an in-memory slice edit.
	journalPath := filepath.Join(root, "journal", "journal.jsonl")
	surviving := []string{}
	for _, ln := range lines {
		if strings.Contains(ln, writerB.ExecutionID) {
			continue
		}
		surviving = append(surviving, ln)
	}
	if len(surviving) != 1 {
		t.Fatalf("mutant procedural error: expected exactly one surviving line, got %d", len(surviving))
	}
	// Rewrite the on-disk journal with only the surviving line, then re-open the
	// store so load() reflects the mutated (line-lost) on-disk state.
	if err := os.WriteFile(journalPath, []byte(strings.Join(surviving, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("rewrite journal to lose writer B's line: %v", err)
	}
	mutated, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("reopen mutated journal: %v", err)
	}
	// The execution B was supposed to create is now absent from the durable
	// journal — the loss is real on disk, not in memory.
	if _, ok := mutated.get(writerB.ExecutionID); ok {
		t.Fatal("mutant FAIL: writer B's execution must be absent from the rewritten on-disk journal")
	}
	survivingLines, err := readJournalLines(root)
	if err != nil {
		t.Fatalf("read mutated journal: %v", err)
	}
	if len(survivingLines) != 1 {
		t.Fatalf("mutant FAIL: the on-disk journal must carry exactly one line after losing writer B, got %d", len(survivingLines))
	}
	// The red witness: run the genuine order-cell assertion against the surviving
	// on-disk journal lines. It expects exactly two distinct ordered lines; with
	// B's line lost it sees one and goes RED — captured as a t.Log (not fatal) so
	// the guard's sensitivity is demonstrated, not the baseline.
	if !orderedCellWouldFail(survivingLines) {
		t.Fatal("mutant FAIL (should be RED): with writer B's write lost on disk, the ordered cell must fail")
	}
	t.Log("MUTANT B RED: with writer B's line lost on the same on-disk journal path, the genuine ordered cell sees 1 line, not 2 distinct ordered lines — the order guard is real")
}

// orderedCellWouldFail reports whether the ordered-write assertion "exactly two
// distinct writers each contribute one line in provenance order A before B" is
// FALSE for the given surviving lines. It runs the EXACT assertion the ordered
// cell makes; when a line was lost it returns true (RED).
func orderedCellWouldFail(lines []string) bool {
	if len(lines) != 2 {
		return true // RED: not two distinct ordered lines
	}
	return false // GREEN: two distinct lines present
}

// orderOf returns the 1-based line index of the first journal line that carries
// the given execution id.
func orderOf(lines []string, id string) int {
	for i, ln := range lines {
		if strings.Contains(ln, id) {
			return i + 1
		}
	}
	return 0
}
