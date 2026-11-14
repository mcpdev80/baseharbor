package deployment

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAbsentFirstDeploymentDoesNotRequireCoreOwner(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	identity := testDeploymentIdentity(t, "local", "first-app", "dev")
	if _, err := LoadDeploymentRecord(identity); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("absent first deployment should be reported as not found, got %v", err)
	}
}

func TestLoadIncompleteDeploymentStillRequiresCoreOwner(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	identity := testDeploymentIdentity(t, "local", "incomplete-app", "dev")
	root, err := DeploymentRoot(identity)
	if err != nil {
		t.Fatalf("resolve deployment root: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o700); err != nil {
		t.Fatalf("create incomplete deployment namespace: %v", err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatalf("create incomplete deployment directory: %v", err)
	}

	_, err = LoadDeploymentRecord(identity)
	if !errors.Is(err, ErrDeploymentOwnerMissing) {
		t.Fatalf("existing incomplete state must still require an authoritative owner, got %v", err)
	}
}
