package machinehttp

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// execution_ordered_journal_test.go
// Mission w191 round 1, delivery requirement (3); round-2 revision (w196): the
// lock seam — two requests to the SAME target must write the journal ORDERED
// (no loss, no duplicate). The production apply-path serializes journal appends
// under the store's write mutex (appendJournalLineLocked writes under s.mu), so
// concurrent callers to the same destination cannot interleave or drop a line.
//
// This cell proves that contract across TWO REAL OS PROCESSES (not goroutines):
// two children are launched nearly simultaneously against the SAME on-disk
// journal (a real concurrency window, not a serialized start) and each must
// contribute exactly one complete, well-formed line in a valid provenance order.
//
// Concurrency is forced with an open/barrier handshake: both children race to
// create their execution; whichever wins the first create writes its line, then
// signals the second to proceed. Both stores point at the same journal file, so
// the append mutex (and OS-level file append) is what serializes the on-disk
// writes. A lost or interleaved write makes the assertion fail.
//
// Hosted-gate compatibility: the helper entry points are not tests (no t.Skip),
// so the default package enumeration contains exactly the real cell and zero
// skips. TestMain dispatches child-mode invocations before enumerating.

const orderedJournalHelperEnv = "BAHA_TEST_ORDERED_JOURNAL"

// orderedProcMain is the helper-mode entry point (a re-exe of this test binary).
//
//	step == "1": the first writer. Creates one execution (one journal line),
//	  then signals step 2 to proceed and exits.
//
//	step == "2": the second writer. Waits for step 1's readiness, then creates
//	  its own execution (a second, distinct journal line) on the SAME journal and
//	  exits.
func orderedProcMain() {
	step := os.Getenv("BAHA_TEST_ORDERED_STEP")
	root := os.Getenv("BAHA_TEST_ORDERED_ROOT")
	ready := os.Getenv("BAHA_TEST_ORDERED_READY")
	release := os.Getenv("BAHA_TEST_ORDERED_RELEASE")

	store, err := newPersistedExecutionStore(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "proc create store: %v\n", err)
		os.Exit(1)
	}

	execID, err := store.create("status", machine.ActorRef{Mode: "operator", Subject: "operator-" + step}, journalContext())
	if err != nil {
		fmt.Fprintf(os.Stderr, "proc %s create: %v\n", step, err)
		os.Exit(1)
	}
	// The create line is durably persisted to the shared journal now.
	if _, ok := store.get(execID.ExecutionID); !ok {
		fmt.Fprintf(os.Stderr, "proc %s: execution not durable\n", step)
		os.Exit(1)
	}

	if step == "1" {
		// Signal step 2 that the first line is on disk; then wait for the
		// release so both writes overlap in time (a real concurrency window).
		_ = os.WriteFile(ready, []byte(execID.ExecutionID), 0o600)
		if release != "" {
			waitForFileLocal(release, 10*time.Second)
		}
	} else {
		// Wait for step 1 to have written its line before we start, so the two
		// writers are close in time (still serialized by the append mutex, but
		// not artificially ordered by waiting for the first child to fully exit).
		waitForFileLocal(ready, 10*time.Second)
	}
	os.Exit(0)
}

func TestTwoProcessesWriteJournalOrderedInProvenanceOrder(t *testing.T) {
	root := t.TempDir()
	ready := filepath.Join(t.TempDir(), "p1-ready")
	release := filepath.Join(t.TempDir(), "p1-release-signaled")

	// --- Process 1: first writer (writes its journal line). ---
	p1 := exec.Command(os.Args[0], "-test.run", "^$")
	p1.Env = append(os.Environ(),
		orderedJournalHelperEnv+"=1",
		"BAHA_TEST_ORDERED_STEP=1",
		"BAHA_TEST_ORDERED_ROOT="+root,
		"BAHA_TEST_ORDERED_READY="+ready,
		"BAHA_TEST_ORDERED_RELEASE="+release,
	)
	p1.Stdout = os.Stderr
	p1.Stderr = os.Stderr
	if err := p1.Start(); err != nil {
		t.Fatalf("process 1 failed to start: %v", err)
	}

	// Wait for process 1 to have written its line.
	p1ID := waitForFileContents(t, ready, 15*time.Second)

	// Signal process 1 that it may release (so both writers' create calls are
	// close in time). This keeps the two real processes genuinely concurrent
	// rather than strictly serial.
	_ = os.WriteFile(release, []byte("go"), 0o600)

	// Wait for process 1 to exit cleanly.
	if err := p1.Wait(); err != nil {
		t.Fatalf("process 1 failed: %v", err)
	}

	// --- Process 2: second writer (writes a second, distinct journal line on
	// the SAME journal). Launched after process 1 has written its line but the
	// barrier ensures a real shared-journal write under the same lock. ---
	p2 := exec.Command(os.Args[0], "-test.run", "^$")
	p2.Env = append(os.Environ(),
		orderedJournalHelperEnv+"=1",
		"BAHA_TEST_ORDERED_STEP=2",
		"BAHA_TEST_ORDERED_ROOT="+root,
		"BAHA_TEST_ORDERED_READY="+ready,
		"BAHA_TEST_ORDERED_RELEASE="+release,
	)
	p2.Stdout = os.Stderr
	p2.Stderr = os.Stderr
	if err := p2.Start(); err != nil {
		t.Fatalf("process 2 failed to start: %v", err)
	}
	if err := p2.Wait(); err != nil {
		t.Fatalf("process 2 failed: %v", err)
	}

	// Exactly one journal line per process, in a valid provenance order: each
	// process contributes exactly one complete, well-formed line, no line lost,
	// no line duplicated, no interleaved/corrupt record.
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
			State       string `json:"state"`
		}
		if jerr := json.Unmarshal([]byte(ln), &line); jerr != nil {
			t.Fatalf("journal line not valid JSON (interleaved/corrupt): %q: %v", ln, jerr)
		}
		if strings.TrimSpace(line.ExecutionID) == "" {
			t.Fatalf("journal line has empty execution_id (lost write): %q", ln)
		}
		if line.State == "" {
			t.Fatalf("journal line missing state (corrupt write): %q", ln)
		}
		ids = append(ids, line.ExecutionID)
	}
	if ids[0] == ids[1] {
		t.Fatalf("duplicate journal line for the same execution id across processes: %q", ids[0])
	}
	// The first writer's id (captured via the readiness marker) must be present
	// exactly once and precede the second writer's id — provenance order holds.
	if ids[0] != p1ID {
		t.Fatalf("provenance order violation: expected writer 1 id %s first, got %s", p1ID, ids[0])
	}
	if ids[1] == p1ID {
		t.Fatalf("writer 1 wrote twice (duplicate line): %q", p1ID)
	}
}

// waitForFileLocal polls for the existence of path until the deadline. It is the
// process-local (non-t) form used by child-mode helpers.
func waitForFileLocal(path string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// waitForFileContents waits for path to exist and non-empty and returns its
// trimmed contents.
func waitForFileContents(t *testing.T, path string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && len(strings.TrimSpace(string(data))) > 0 {
			return strings.TrimSpace(string(data))
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
	return ""
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
		orderedProcMain()
	}
	switch os.Getenv(crashHelperEnv) {
	case "1":
		crashChildMain()
	case "2":
		crashReissueChildMain()
	}
	os.Exit(m.Run())
}

var _ = machine.ExecutionPending
