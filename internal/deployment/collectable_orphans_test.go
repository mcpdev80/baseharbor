package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
)

// seedOwnerlessOrphan writes a deployment.json under <state>/targets/<name>/deployments
// with no Core installation owner, reproducing a target-delete orphan.
func seedOwnerlessOrphan(t *testing.T, name, deploymentID string) {
	t.Helper()
	root, err := TargetStateRoot(name)
	if err != nil {
		t.Fatal(err)
	}
	depDir := filepath.Join(root, "deployments", deploymentID)
	if err := os.MkdirAll(depDir, 0700); err != nil {
		t.Fatal(err)
	}
	record := DeploymentRecord{
		Version: DeploymentRecordVersion,
		Identity: DeploymentIdentity{
			DeploymentID:  deploymentID,
			ApplicationID: "orphan-app-id-0000-0000-0000-000000000001",
			Target:        name,
			Application:   "orphanapp",
			Environment:   "default",
		},
		Source: DeploymentSource{Kind: "repository", Repository: "/nonexistent/repo", Manifest: "/nonexistent/baseharbor.yaml"},
	}
	if err := os.WriteFile(filepath.Join(depDir, "deployment.json"), func() []byte {
		b, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}(), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCollectableOrphansFindsOwnerlessDeployment(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	name := "orphan-target"
	seedOwnerlessOrphan(t, name, "11111111-1111-4111-8111-111111111111")

	orphans, err := CollectableOrphans(name)
	if err != nil {
		t.Fatalf("CollectableOrphans errored on an ownerless orphan: %v", err)
	}
	if len(orphans) != 1 {
		t.Fatalf("expected exactly 1 collectable orphan, got %d", len(orphans))
	}
	if orphans[0].Identity.DeploymentID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("unexpected orphan identity: %#v", orphans[0].Identity)
	}
	if SourceAvailable(orphans[0].Source) {
		t.Fatal("seeded orphan source is unavailable, but CollectableOrphans reported it available")
	}
}

func TestCollectableOrphansReturnsNilForCleanTarget(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	name := "clean-target"
	orphans, err := CollectableOrphans(name)
	if err != nil {
		t.Fatalf("clean Target (no deployments) should return nil, nil: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("clean Target reported orphans: %#v", orphans)
	}
}

// seedMismatchedUuidOrphan writes a deployment.json whose embedded DeploymentID
// does NOT equal its deployment directory name (a tampered/corrupt record).
func seedMismatchedUuidOrphan(t *testing.T, name string) {
	t.Helper()
	root, err := TargetStateRoot(name)
	if err != nil {
		t.Fatal(err)
	}
	depDir := filepath.Join(root, "deployments", "orphan-folder")
	if err := os.MkdirAll(depDir, 0700); err != nil {
		t.Fatal(err)
	}
	// Embedded UUID deliberately differs from the folder name.
	record := DeploymentRecord{
		Version: DeploymentRecordVersion,
		Identity: DeploymentIdentity{
			DeploymentID:  "22222222-2222-4222-8222-222222222222",
			ApplicationID: "orphan-app-id-0000-0000-0000-000000000001",
			Target:        name,
			Application:   "orphanapp",
			Environment:   "default",
		},
		Source: DeploymentSource{Kind: "repository", Repository: "/nonexistent/repo", Manifest: "/nonexistent/baseharbor.yaml"},
	}
	if err := os.WriteFile(filepath.Join(depDir, "deployment.json"), func() []byte {
		b, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}(), 0600); err != nil {
		t.Fatal(err)
	}
}

// TestCollectableOrphansRejectsUuidFolderMismatch verifies F2: a record whose
// embedded DeploymentID does not match its directory name is excluded from the
// collectable set (it is a corrupt/foreign record, not a genuine orphan).
func TestCollectableOrphansRejectsUuidFolderMismatch(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	name := "mismatch-target"
	seedMismatchedUuidOrphan(t, name)

	orphans, err := CollectableOrphans(name)
	if err != nil {
		t.Fatalf("CollectableOrphans errored on a mismatched orphan: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("mismatched record must NOT be collectable, got %#v", orphans)
	}
}

// TestCollectableOrphansAcceptsMatchingUuidFolder verifies the correct path: a
// record whose embedded UUID DOES equal its directory name is collectable.
func TestCollectableOrphansAcceptsMatchingUuidFolder(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	name := "match-target"
	seedOwnerlessOrphan(t, name, "33333333-3333-4333-8333-333333333333")

	orphans, err := CollectableOrphans(name)
	if err != nil {
		t.Fatalf("CollectableOrphans errored on a matching orphan: %v", err)
	}
	if len(orphans) != 1 {
		t.Fatalf("expected exactly 1 collectable orphan, got %d", len(orphans))
	}
	if orphans[0].Identity.DeploymentID != "33333333-3333-4333-8333-333333333333" {
		t.Fatalf("unexpected orphan identity: %#v", orphans[0].Identity)
	}
}

func TestCollectableOrphansIgnoresOwnedDeployment(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	name := "owned-target"
	// A Core installation under the Target runtime owns its deployments namespace,
	// so a record under it is a normal deployment, not a collectable orphan.
	stateRoot, err := TargetStateRoot(name)
	if err != nil {
		t.Fatal(err)
	}
	installRoot := filepath.Join(stateRoot, "runtime")
	if _, err := coreinstallation.Run(context.Background(), installRoot, coreinstallation.Spec{Target: name, Runtime: "docker", MachineRole: coreinstallation.Development}, coreinstallation.Steps{
		Preflight: func(context.Context, coreinstallation.State) error { return nil },
		SQL:       func(context.Context, coreinstallation.State) error { return nil },
		Secrets:   func(context.Context, coreinstallation.State) error { return nil },
		Identity: func(context.Context, coreinstallation.State) (string, error) {
			return "https://identity.localhost:8443/realms/baseharbor", nil
		},
		Verify: func(context.Context, coreinstallation.State) error { return nil },
	}); err != nil {
		t.Fatalf("seed a genuine Core installation: %v", err)
	}
	seedOwnerlessOrphan(t, name, "22222222-2222-4222-8222-222222222222")
	orphans, err := CollectableOrphans(name)
	if err != nil {
		t.Fatalf("owned deployment should not be reported: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("owned deployment should not be a collectable orphan, got %#v", orphans)
	}
}

// seedBlankIdentityOrphan writes a deployment.json whose embedded DeploymentID or
// Target is missing or whitespace-only. The r1 decode-uuid fix only rejected
// non-empty mismatches, so a blank field slipped through and was advertised as
// collectable — a blank identity cannot satisfy exact canonical equality with the
// folder/target name and must be excluded.
func seedBlankIdentityOrphan(t *testing.T, name, deploymentID, blankDeploymentID, blankTarget string) {
	t.Helper()
	root, err := TargetStateRoot(name)
	if err != nil {
		t.Fatal(err)
	}
	depDir := filepath.Join(root, "deployments", deploymentID)
	if err := os.MkdirAll(depDir, 0700); err != nil {
		t.Fatal(err)
	}
	record := DeploymentRecord{
		Version: DeploymentRecordVersion,
		Identity: DeploymentIdentity{
			DeploymentID:  blankDeploymentID,
			ApplicationID: "orphan-app-id-0000-0000-0000-000000000001",
			Target:        blankTarget,
			Application:   "orphanapp",
			Environment:   "default",
		},
		Source: DeploymentSource{Kind: "repository", Repository: "/nonexistent/repo", Manifest: "/nonexistent/baseharbor.yaml"},
	}
	if err := os.WriteFile(filepath.Join(depDir, "deployment.json"), func() []byte {
		b, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}(), 0600); err != nil {
		t.Fatal(err)
	}
}

// TestCollectableOrphansRejectsBlankDeploymentID verifies F3: a record whose
// embedded DeploymentID is blank (empty or whitespace) is excluded from the
// collectable set — a blank field cannot prove its identity equals the folder.
func TestCollectableOrphansRejectsBlankDeploymentID(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	name := "blank-uuid-target"
	seedBlankIdentityOrphan(t, name, "44444444-4444-4444-8444-444444444444", "   ", name)

	orphans, err := CollectableOrphans(name)
	if err != nil {
		t.Fatalf("CollectableOrphans errored on a blank-DeploymentID orphan: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("blank DeploymentID record must NOT be collectable, got %#v", orphans)
	}
}

// TestCollectableOrphansRejectsBlankTarget verifies F3: a record whose embedded
// Target is blank (empty or whitespace) is excluded from the collectable set —
// it does not prove it belongs to the detached target.
func TestCollectableOrphansRejectsBlankTarget(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	name := "blank-target-target"
	seedBlankIdentityOrphan(t, name, "55555555-5555-4555-8555-555555555555", name, "   ")

	orphans, err := CollectableOrphans(name)
	if err != nil {
		t.Fatalf("CollectableOrphans errored on a blank-Target orphan: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("blank Target record must NOT be collectable, got %#v", orphans)
	}
}

var _ = errors.Is

// seedMissingFieldOrphan writes a deployment.json whose embedded DeploymentID
// field is entirely absent from the JSON body. JSON unmarshalling leaves the
// decoded string zero, so the embedded identity is empty and cannot equal the
// folder/target name. Without an explicit missing-field fixture the blank
// guard could be satisfied only by the whitespace-only body fixture, never by a
// structurally absent field — the exact shape the identity filter must reject.
func seedMissingFieldOrphan(t *testing.T, name, folderID string) {
	t.Helper()
	root, err := TargetStateRoot(name)
	if err != nil {
		t.Fatal(err)
	}
	depDir := filepath.Join(root, "deployments", folderID)
	if err := os.MkdirAll(depDir, 0700); err != nil {
		t.Fatal(err)
	}
	// A deliberately partial record: only ApplicationID and a version are
	// written, so deployment_id and target are missing from the body.
	record := DeploymentRecord{
		Version: DeploymentRecordVersion,
		Identity: DeploymentIdentity{
			ApplicationID: "orphan-app-id-0000-0000-0000-000000000001",
		},
		Source: DeploymentSource{Kind: "repository", Repository: "/nonexistent/repo", Manifest: "/nonexistent/baseharbor.yaml"},
	}
	if err := os.WriteFile(filepath.Join(depDir, "deployment.json"), func() []byte {
		b, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}(), 0600); err != nil {
		t.Fatal(err)
	}
}

// TestCollectableOrphansRejectsMissingDeploymentIDField verifies F3 for a record
// whose DeploymentID is structurally absent from the JSON body: the decoded
// identity is empty and cannot equal the folder name, so the record is excluded
// from the collectable set.
func TestCollectableOrphansRejectsMissingDeploymentIDField(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	name := "missing-field-target"
	seedMissingFieldOrphan(t, name, "66666666-6666-4666-8666-666666666666")

	orphans, err := CollectableOrphans(name)
	if err != nil {
		t.Fatalf("CollectableOrphans errored on a missing-DeploymentID orphan: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("missing-DeploymentID record must NOT be collectable, got %#v", orphans)
	}
}

// TestCollectableOrphansRejectsMissingTargetField verifies F3 for a record whose
// Target field is structurally absent from the JSON body: the decoded identity
// is empty and cannot equal the detached target, so the record is excluded.
func TestCollectableOrphansRejectsMissingTargetField(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	name := "missing-target-field-target"
	seedMissingFieldOrphan(t, name, "77777777-7777-4777-8777-777777777777")

	orphans, err := CollectableOrphans(name)
	if err != nil {
		t.Fatalf("CollectableOrphones errored on a missing-Target orphan: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("missing-Target record must NOT be collectable, got %#v", orphans)
	}
}

// seedPaddedIdentityOrphan writes a deployment.json whose embedded DeploymentID
// and Target are equal to the folder/target after trimming surrounding
// whitespace, but not equal raw. The corrected identity filter rejects such a
// record: a value that is equal only after stripping whitespace does not prove
// its identity as a canonical UUID/Target and must not be advertised as
// collectable.
func seedPaddedIdentityOrphan(t *testing.T, name, folderID string) {
	t.Helper()
	root, err := TargetStateRoot(name)
	if err != nil {
		t.Fatal(err)
	}
	depDir := filepath.Join(root, "deployments", folderID)
	if err := os.MkdirAll(depDir, 0700); err != nil {
		t.Fatal(err)
	}
	record := DeploymentRecord{
		Version: DeploymentRecordVersion,
		Identity: DeploymentIdentity{
			DeploymentID:  " " + folderID + " ",
			ApplicationID: "orphan-app-id-0000-0000-0000-000000000001",
			Target:        " " + name + " ",
			Application:   "orphanapp",
			Environment:   "default",
		},
		Source: DeploymentSource{Kind: "repository", Repository: "/nonexistent/repo", Manifest: "/nonexistent/baseharbor.yaml"},
	}
	if err := os.WriteFile(filepath.Join(depDir, "deployment.json"), func() []byte {
		b, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}(), 0600); err != nil {
		t.Fatal(err)
	}
}

// TestCollectableOrphansRejectsPaddedValues verifies that a record whose
// DeploymentID and Target are equal to the folder/target only after trimming is
// NOT collectable: the raw comparison rejects non-canonical values.
func TestCollectableOrphansRejectsPaddedValues(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	name := "padded-target"
	seedPaddedIdentityOrphan(t, name, "88888888-8888-4888-8888-888888888888")

	orphans, err := CollectableOrphans(name)
	if err != nil {
		t.Fatalf("CollectableOrphones errored on a padded-value orphan: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("whitespace-padded record must NOT be collectable, got %#v", orphans)
	}
}

// TestCollectableOrphansAcceptsCanonicalValues verifies the correct path: a
// record whose embedded UUID and Target are byte-for-byte equal to the folder
// and detached target is collectable, proving the raw comparison does not over-
// reject genuine orphans.
func TestCollectableOrphansAcceptsCanonicalValues(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	name := "canonical-target"
	seedOwnerlessOrphan(t, name, "99999999-9999-4999-8999-999999999999")

	orphans, err := CollectableOrphans(name)
	if err != nil {
		t.Fatalf("CollectableOrphones errored on a canonical orphan: %v", err)
	}
	if len(orphans) != 1 {
		t.Fatalf("expected exactly 1 collectable orphan, got %d", len(orphans))
	}
	if orphans[0].Identity.DeploymentID != "99999999-9999-4999-8999-999999999999" {
		t.Fatalf("unexpected orphan identity: %#v", orphans[0].Identity)
	}
	if orphans[0].Identity.Target != name {
		t.Fatalf("unexpected orphan target: %#v", orphans[0].Identity)
	}
}
