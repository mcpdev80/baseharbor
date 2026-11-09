package deployment

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSaveDeploymentRecordPropagatesBandWriteError proves the r10 fail-closed
// criterion: a band write failure inside the production save seam must surface
// as a SaveDeploymentRecord error, never a silent success. A successful save
// that omits the durable ownership marker would let an up/edit run report
// success while the resolver lacks its trusted identity source — exactly the
// "write error looks like success" defect the r7 recipe forbids. The failure is
// injected by pre-creating RetainedOwnershipBandName as a directory: the
// atomic rename of the band tmp file onto that path must fail.
func TestSaveDeploymentRecordPropagatesBandWriteError(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	seedManagedTargetOwner(t, "default")

	id := testDeploymentIdentity(t, "default", "band-writer", "dev")
	seedAuthoritativeCoreOwnerForTarget(t, "default")

	root, rootErr := DeploymentRoot(id)
	if rootErr != nil {
		t.Fatalf("resolve deployment root: %v", rootErr)
	}

	// Pre-create the band path as a non-empty directory so the atomic rename
	// below fails (rename of a file onto a non-empty directory is EEXIST/EISDIR).
	badBand := filepath.Join(root, RetainedOwnershipBandName)
	if mkErr := os.MkdirAll(badBand, 0o700); mkErr != nil {
		t.Fatalf("seed unwritable band path: %v", mkErr)
	}

	// The deployment.json must still persist even though the band write fails
	// afterward — but the save as a whole must be reported as failed, never
	// success, because the trusted ownership marker is absent.
	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository", Repository: "/tmp/band-writer/baseharbor.yaml", Manifest: "/tmp/band-writer/baseharbor.yaml"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker"},
	}
	saveErr := SaveDeploymentRecord(record)
	if saveErr == nil {
		t.Fatalf("SaveDeploymentRecord must fail when the band cannot be written; got success")
	}
	if _, statErr := os.Stat(badBand); statErr != nil {
		t.Fatalf("deployment.json must have persisted before the band write: %v", statErr)
	}
}
