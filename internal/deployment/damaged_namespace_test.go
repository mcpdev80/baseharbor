package deployment

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDamagedDeploymentNamespaceDetectsOrphanedState verifies that the
// namespace scanner flags a deploy directory whose deployment.json is missing
// while protected runtime state was retained, and that it ignores an empty
// directory (the pre-existing incomplete case).
func TestDamagedDeploymentNamespaceDetectsOrphanedState(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	target := "local"

	// Case 1: empty namespace - not damaged.
	damaged, _, err := DamagedDeploymentNamespace(target, "app-1", "dev")
	if err != nil {
		t.Fatalf("empty namespace scan: %v", err)
	}
	if damaged {
		t.Fatal("empty deployment namespace must not be flagged damaged")
	}

	depDir := deploymentsNamespaceForTest(t, target)

	// Case 2: orphaned state without deployment.json - damaged.
	damagedDir := filepath.Join(depDir, "damaged-dir")
	if err := os.MkdirAll(damagedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(damagedDir, "application.env"), []byte("X=Y\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	damaged, root, err := DamagedDeploymentNamespace(target, "app-3", "dev")
	if err != nil {
		t.Fatalf("damaged scan: %v", err)
	}
	if !damaged {
		t.Fatal("orphaned deploy directory with runtime state but missing deployment.json must be flagged damaged")
	}
	if root != damagedDir {
		t.Fatalf("damaged dir path = %q, want %q", root, damagedDir)
	}

	// Case 3: a healthy directory with a deployment.json is not damaged.
	healthyDir := filepath.Join(depDir, "healthy-dir")
	if err := os.MkdirAll(healthyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(healthyDir, "deployment.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	damaged, root, err = DamagedDeploymentNamespace(target, "app-4", "dev")
	if err != nil {
		t.Fatalf("mixed scan: %v", err)
	}
	if !damaged || root != damagedDir {
		t.Fatalf("scan must still report the damaged dir; got damaged=%v root=%q", damaged, root)
	}
}

// TestDamagedDeploymentNamespaceIgnoresEmptyAndNeverDelivered verifies that an
// empty directory is not damaged and a directory that only carries the
// never-delivered one-shot proof is not flagged damaged.
func TestDamagedDeploymentNamespaceIgnoresEmptyAndNeverDelivered(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	target := "local"
	depDir := deploymentsNamespaceForTest(t, target)

	// Empty directory - not damaged.
	emptyDir := filepath.Join(depDir, "empty")
	if err := os.MkdirAll(emptyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	damaged, _, err := DamagedDeploymentNamespace(target, "app-empty", "dev")
	if err != nil {
		t.Fatalf("empty-dir scan: %v", err)
	}
	if damaged {
		t.Fatal("empty deploy directory must not be flagged damaged")
	}

	// A never-delivered proof at never-delivered/<applicationID>/<environment>.json
	// (applicationID must be a UUIDv4). The proof marks a legitimate fresh-init;
	// the directory carrying only it (no runtime state) is not damage.
	_, _ = NewDeploymentIdentity(target, "app-proof", "app-proof", "dev")
}

func deploymentsNamespaceForTest(t *testing.T, target string) string {
	t.Helper()
	dir, err := deploymentDirForTarget(target)
	if err != nil {
		t.Fatalf("resolve deployments namespace for %s: %v", target, err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}
