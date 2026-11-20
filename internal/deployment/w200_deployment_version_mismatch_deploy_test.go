package deployment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestListDeploymentsForDisplaySurfacesVersionMismatch proves the best-effort
// display list surfaces a deployable-version-mismatch warning for a
// version-1 deployment.json (written by an older shipped BaseHarbor) on an
// owned target, and names the found/supported versions, instead of hiding the
// record behind a generic empty inventory. This is the seam 'baha app list'
// renders.
func TestListDeploymentsForDisplaySurfacesVersionMismatch(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", dataHome)

	target := "local"
	installRoot := installRuntimeRoot(t, target)
	writeInstallationAuthoritative(t, target, installRoot)

	identity := DeploymentIdentity{
		DeploymentID:  "33333333-3333-4333-8333-333333333333",
		ApplicationID: "app-oldlist-0001",
		Target:        target,
		Application:   "oldlist",
		Environment:   "dev",
	}
	if err := ValidateDeploymentID(identity.DeploymentID); err != nil {
		t.Fatal(err)
	}
	root, err := DeploymentRoot(identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	record := DeploymentRecord{
		Version:  1,
		Identity: identity,
		Source:   DeploymentSource{Kind: "repository", Repository: "/nonexistent"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker"},
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "deployment.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, warns, listErr := ListDeploymentsForDisplay(target)
	if listErr != nil {
		t.Fatalf("list must not abort on a per-record warning: %v", listErr)
	}
	var found bool
	for _, w := range warns {
		st, ok := DeploymentRecordState(w)
		if !ok || st.Kind != "deployable_version_mismatch" {
			continue
		}
		found = true
		if msg := w.Error(); !strings.Contains(msg, "version 1") || !strings.Contains(msg, "version 2") {
			t.Fatalf("list warning must name the found and supported versions: %q", msg)
		}
		// The record bytes are left untouched (fail-closed): nothing migrated.
		if st.Identity.DeploymentID != identity.DeploymentID {
			t.Fatalf("version mismatch must not rewrite the record identity: %+v", st.Identity)
		}
	}
	if !found {
		t.Fatalf("list must surface a deployable_version_mismatch warning: %#v", warns)
	}
}

// installRuntimeRoot resolves the Core installation runtime root for a target
// and creates the directory.
func installRuntimeRoot(t *testing.T, target string) string {
	t.Helper()
	targetRoot, err := TargetStateRoot(target)
	if err != nil {
		t.Fatalf("resolve target root for %s: %v", target, err)
	}
	installRoot := filepath.Join(targetRoot, "runtime")
	if err := os.MkdirAll(installRoot, 0o700); err != nil {
		t.Fatalf("create installation runtime root: %v", err)
	}
	return installRoot
}
