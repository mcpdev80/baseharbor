package machinehttp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// TestReviewProbeCompleteJSONWithoutFinalNewlineFailsClosed (Luna Befund-2 / recipe B2)
//
// A syntactically COMPLETE pending JSON line WITHOUT its terminating newline must
// be treated as corrupt/uncertain on load — NOT silently restored as pending, even
// though its bytes are individually valid JSON. The shipped TestRound7CrashHalfWrite
// appends INVALID JSON; this probe writes VALID-but-unterminated JSON.
func TestReviewProbeCompleteJSONWithoutFinalNewlineFailsClosed(t *testing.T) {
	root := t.TempDir()
	journalDir := filepath.Join(root, "journal")
	if err := os.MkdirAll(journalDir, 0o700); err != nil {
		t.Fatalf("mkdir journal: %v", err)
	}
	activePath := filepath.Join(journalDir, "journal.jsonl")

	// A committed running line (with newline).
	validLine, err := json.Marshal(struct {
		ExecutionID string                 `json:"execution_id"`
		OperationID string                 `json:"operation_id"`
		State       machine.ExecutionState `json:"state"`
		StartedAt   *time.Time             `json:"started_at,omitempty"`
	}{
		ExecutionID: "complete-exec",
		OperationID: "status",
		State:       machine.ExecutionRunning,
	})
	if err != nil {
		t.Fatalf("marshal valid line: %v", err)
	}
	if err := os.WriteFile(activePath, append(validLine, '\n'), 0o600); err != nil {
		t.Fatalf("write valid line: %v", err)
	}
	// A syntactically COMPLETE pending JSON line WITHOUT the terminating newline.
	completeNoNL, err := json.Marshal(struct {
		ExecutionID string                 `json:"execution_id"`
		OperationID string                 `json:"operation_id"`
		State       machine.ExecutionState `json:"state"`
	}{
		ExecutionID: "complete-exec",
		OperationID: "status",
		State:       machine.ExecutionPending,
	})
	if err != nil {
		t.Fatalf("marshal complete line: %v", err)
	}
	// No trailing newline => the append was never committed as a JSONL record.
	if err := os.WriteFile(activePath, completeNoNL, 0o600); err != nil {
		t.Fatalf("append complete-no-newline: %v", err)
	}

	_, loadErr := newPersistedExecutionStore(root)
	if loadErr == nil {
		t.Fatal("FAIL (B2): complete pending JSON without newline was silently accepted (would restore as pending/running)")
	}
}
