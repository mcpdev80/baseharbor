package openbao

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestReserveRecoveryFileAllowsSiblingOfExplicitStateDir(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, "baseharbor-runtime")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	files := bhruntime.Files{
		Compose: filepath.Join(stateDir, "compose.yaml"),
		Env:     filepath.Join(stateDir, "runtime.env"),
	}
	recovery := filepath.Join(root, "openbao-recovery.json")

	path, file, err := reserveRecoveryFile(files, recovery)
	if err != nil {
		t.Fatal(err)
	}
	if path != recovery {
		t.Fatalf("recovery path = %q, want %q", path, recovery)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestReserveRecoveryFileRejectsInsideExplicitStateDir(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, "custom-state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	files := bhruntime.Files{
		Compose: filepath.Join(stateDir, "compose.yaml"),
		Env:     filepath.Join(stateDir, "runtime.env"),
	}

	_, _, err := reserveRecoveryFile(files, filepath.Join(stateDir, "recovery.json"))
	if err == nil || !strings.Contains(err.Error(), "outside BaseHarbor state") {
		t.Fatalf("error = %v, want state-boundary rejection", err)
	}
}

func TestReserveRecoveryFileStillProtectsLegacyBaseHarborRoot(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, ".baseharbor", "runtime")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	files := bhruntime.Files{
		Compose: filepath.Join(stateDir, "compose.yaml"),
		Env:     filepath.Join(stateDir, "runtime.env"),
	}

	_, _, err := reserveRecoveryFile(files, filepath.Join(root, ".baseharbor", "recovery.json"))
	if err == nil || !strings.Contains(err.Error(), "outside BaseHarbor state") {
		t.Fatalf("error = %v, want legacy .baseharbor boundary rejection", err)
	}
}
