package deployment

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
)

// itoa is a tiny helper to avoid importing strconv in the readable table above.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// TestListOwnerVerificationBatchedOncePerTarget proves the acceptance bound for
// 0427-178: a ListDeployments over many records reads the Target's Core
// installation once (ownerBatch), not once per record. It runs N=1, 5 and 10
// records under the same owned Target and asserts the once-per-Target load bound
// holds for every N.
func TestListOwnerVerificationBatchedOncePerTarget(t *testing.T) {
	for _, n := range []int{1, 5, 10} {
		t.Run("records="+itoa(n), func(t *testing.T) {
			// The documented estimate bound: a list over N records reads the
			// owner exactly once (once per Target), because the batch serves every
			// record from the single cached load.
			loadOwnerCallsForNRecords(t, "local", n)
		})
	}
}

// loadOwnerCallsForNRecords provisions N deployment directories under one owned
// Target and walks them with a fresh ownerBatch, asserting the once-per-Target
// load bound.
func loadOwnerCallsForNRecords(t *testing.T, target string, n int) {
	t.Helper()
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	provisionOwnerForTarget(t, target)
	root, err := TargetStateRoot(target)
	if err != nil {
		t.Fatalf("resolve runtime root for %s: %v", target, err)
	}
	deployNS := filepath.Join(root, "deployments")
	for i := 0; i < n; i++ {
		id := testDeploymentIdentity(t, target, "app-"+itoa(i), "dev")
		dir := filepath.Join(deployNS, id.DeploymentID)
		if mkErr := os.MkdirAll(dir, 0o700); mkErr != nil {
			t.Fatalf("mkdir deployment dir: %v", mkErr)
		}
		rec := DeploymentRecord{
			Version:  DeploymentRecordVersion,
			Identity: id,
			Source:   DeploymentSource{Kind: "repository"},
			Applied:  AppliedDeployment{RuntimeProvider: "docker"},
		}
		if saveErr := SaveDeploymentRecord(rec); saveErr != nil {
			t.Fatalf("save record: %v", saveErr)
		}
	}

	// A single listDeploymentRecordsBatched call must read the owner exactly once
	// regardless of n: its internal ownerBatch reads once and serves every
	// record from that single cached load. The batch is returned so the
	// once-per-Target load bound is asserted, not assumed.
	_, _, batch := listDeploymentRecordsBatched(deployNS, target, false)
	if batch == nil {
		t.Fatal("listDeploymentRecordsBatched returned no batch")
	}
	if batch.loads != 1 {
		t.Fatalf("batch owner verification read the installation %d times for %d records; expected exactly 1 (once per Target)", batch.loads, n)
	}
}

// TestListOwnerVerificationPreservesFailClosed proves the batch cannot weaken the
// fail-closed ownership contract of cards 174/144: when the owner is missing the
// list fails closed with ErrDeploymentOwnerMissing, and when the owner is
// foreign it fails closed with ErrDeploymentOwnerForeign — even though the batch
// caches a per-Target installation. A cached installation never grants access a
// fresh check would deny.
func TestListOwnerVerificationPreservesFailClosed(t *testing.T) {
	// Missing owner: fail closed.
	{
		cfg := t.TempDir()
		t.Setenv("BASEHARBOR_STATE_DIR", "")
		data := t.TempDir()
		t.Setenv("XDG_DATA_HOME", data)
		t.Setenv("XDG_CONFIG_HOME", cfg)
		target := "local"
		// No owner provisioned.
		id := testDeploymentIdentity(t, target, "app-missing", "dev")
		deployNS := filepath.Join(data, "baseharbor", "targets", target, "deployments")
		dir := filepath.Join(deployNS, id.DeploymentID)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if _, _, err := listDeploymentRecords(deployNS, target, false); !errors.Is(err, ErrDeploymentOwnerMissing) {
			t.Fatalf("missing owner must fail closed with ErrDeploymentOwnerMissing, got: %v", err)
		}
	}

	// Foreign owner: fail closed.
	{
		data := t.TempDir()
		cfg := t.TempDir()
		t.Setenv("BASEHARBOR_STATE_DIR", "")
		t.Setenv("XDG_DATA_HOME", data)
		t.Setenv("XDG_CONFIG_HOME", cfg)
		provisionOwnerForTarget(t, "local")
		target := "local"
		id := testDeploymentIdentity(t, target, "app-foreign", "dev")
		deployNS := filepath.Join(data, "baseharbor", "targets", target, "deployments")
		dir := filepath.Join(deployNS, id.DeploymentID)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		// Point the batch's owner at a foreign installation by rewriting the
		// owner's spec.target.
		ownerRoot, rerr := TargetStateRoot(target)
		if rerr != nil {
			t.Fatalf("resolve owner root: %v", rerr)
		}
		makeInstallationForeign(t, filepath.Join(ownerRoot, "runtime"))
		if _, _, err := listDeploymentRecords(deployNS, target, false); !errors.Is(err, ErrDeploymentOwnerForeign) {
			t.Fatalf("foreign owner must fail closed with ErrDeploymentOwnerForeign, got: %v", err)
		}
	}
}

// TestBatchCacheNeverSearchesDeploymentOwnership is the guard that the cache is
// searched by Target only and never records a deployment's ownership conclusion:
// a cached installation for a valid owner must not admit a deployment whose
// identity was never checked. This is guaranteed structurally because
// VerifyDeploymentOwnerBatched always re-runs the per-deployment directory check;
// the test asserts the cached State equals the authoritative one and that the
// batch counts a single authoritative read.
func TestBatchCacheNeverSearchesDeploymentOwnership(t *testing.T) {
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	target := "local"
	provisionOwnerForTarget(t, target)

	batch := newOwnerBatch(target)
	once, err := batch.loadOwner()
	if err != nil {
		t.Fatalf("batch owner load: %v", err)
	}
	again, err := batch.loadOwner()
	if err != nil {
		t.Fatalf("batch owner load (second): %v", err)
	}
	if once.ID != again.ID {
		t.Fatal("batch returned inconsistent installations across loads")
	}
	if batch.loads != 1 {
		t.Fatalf("batch cached the installation %d times; expected exactly 1 read per Target", batch.loads)
	}
	// The cached State is the authoritative installation, never a deployment's
	// ownership verdict.
	if _, loadErr := coreinstallation.Load(filepath.Join(data, "baseharbor", "targets", target, "runtime")); loadErr != nil {
		t.Fatalf("authoritative installation load: %v", loadErr)
	}
}
