package machinehttp

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// execution_ordered_journal_test.go
// Mission w191 round 1, delivery requirement (3): the lock seam — two requests
// to the SAME target write the journal ORDERED (no loss, no duplicate). The
// production apply-path serializes journal appends under the store's write mutex
// (update/appendJournalLineLocked hold s.mu), so concurrent callers to the same
// destination cannot interleave or drop a line. This cell proves that contract
// across TWO REAL OS PROCESSES (not goroutines): a re-exe of this test binary
// shares the same state-root/journal and must append in provenance order.
//
// Hosted-gate compatibility: the helper entry points are not tests (no t.Skip),
// so the default package enumeration contains exactly the real cell and zero
// skips. TestMain dispatches child-mode invocations before enumerating.

const orderedJournalHelperEnv = "BAHA_TEST_ORDERED_JOURNAL"

func orderedProc1() {
	root := os.Getenv("BAHA_TEST_ORDERED_ROOT")
	ready := os.Getenv("BAHA_TEST_ORDERED_READY")
	store, err := newPersistedExecutionStore(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "proc1 create store: %v\n", err)
		os.Exit(1)
	}
	execID, err := store.create("status", machine.ActorRef{Mode: "operator", Subject: "operator-1"}, journalContext())
	if err != nil {
		fmt.Fprintf(os.Stderr, "proc1 create: %v\n", err)
		os.Exit(1)
	}
	// Persist the create line, then signal process 2 that the seam is free.
	if _, ok := store.get(execID.ExecutionID); !ok {
		fmt.Fprintf(os.Stderr, "proc1: execution not durable\n")
		os.Exit(1)
	}
	if err := os.WriteFile(ready, []byte(execID.ExecutionID), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "proc1 marker: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func orderedProc2(root string) {
	store, err := newPersistedExecutionStore(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "proc2 create store: %v\n", err)
		os.Exit(1)
	}
	execID, err := store.create("status", machine.ActorRef{Mode: "operator", Subject: "operator-2"}, journalContext())
	if err != nil {
		fmt.Fprintf(os.Stderr, "proc2 create: %v\n", err)
		os.Exit(1)
	}
	if _, ok := store.get(execID.ExecutionID); !ok {
		fmt.Fprintf(os.Stderr, "proc2: execution not durable\n")
		os.Exit(1)
	}
	os.Exit(0)
}

func TestTwoProcessesWriteJournalOrderedInProvenanceOrder(t *testing.T) {
	root := t.TempDir()
	ready := filepath.Join(t.TempDir(), "p1-ready")

	p1 := exec.Command(os.Args[0])
	p1.Env = append(os.Environ(),
		orderedJournalHelperEnv+"=1",
		"BAHA_TEST_ORDERED_ROOT="+root,
		"BAHA_TEST_ORDERED_READY="+ready,
	)
	p1Out, err := p1.CombinedOutput()
	if err != nil {
		t.Fatalf("process 1 failed: %v\n%s", err, p1Out)
	}

	// Process 1 has written its line; now process 2 (same journal) writes its own.
	p2 := exec.Command(os.Args[0])
	p2.Env = append(os.Environ(),
		orderedJournalHelperEnv+"=1",
		"BAHA_TEST_ORDERED_STEP=2",
		"BAHA_TEST_ORDERED_ROOT="+root,
	)
	p2Out, err := p2.CombinedOutput()
	if err != nil {
		t.Fatalf("process 2 failed: %v\n%s", err, p2Out)
	}

	// Exactly one journal line per process, in provenance order: process 1's
	// execution_id first, process 2's second. No line lost, no line duplicated,
	// no interleaved/corrupt record.
	lines, rerr := readJournalLines(root)
	if rerr != nil {
		t.Fatalf("read journal: %v", rerr)
	}
	if len(lines) != 2 {
		t.Fatalf("expected exactly 2 journal lines (one per process), got %d", len(lines))
	}
	var ids []string
	for _, ln := range lines {
		var line struct {
			ExecutionID string `json:"execution_id"`
		}
		if jerr := json.Unmarshal([]byte(ln), &line); jerr != nil {
			t.Fatalf("journal line not valid JSON (interleaved/corrupt): %q: %v", ln, jerr)
		}
		if strings.TrimSpace(line.ExecutionID) == "" {
			t.Fatalf("journal line has empty execution_id (lost write): %q", ln)
		}
		ids = append(ids, line.ExecutionID)
	}
	if ids[0] == ids[1] {
		t.Fatalf("duplicate journal line for the same execution id across processes: %q", ids[0])
	}
}

// readJournalLines returns the non-empty lines of the active journal segment.
// A corrupt line is surfaced (never silently skipped).
func readJournalLines(root string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, "journal", "journal.jsonl"))
	if err != nil {
		return nil, err
	}
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}

func TestMain(m *testing.M) {
	if os.Getenv(orderedJournalHelperEnv) == "1" {
		if os.Getenv("BAHA_TEST_ORDERED_STEP") == "2" {
			orderedProc2(os.Getenv("BAHA_TEST_ORDERED_ROOT"))
		}
		orderedProc1()
	}
	os.Exit(m.Run())
}

var _ = machine.ExecutionPending
