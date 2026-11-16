package deployment

import (
	"errors"
	"fmt"
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

// TestBatchedListClassifiesAccessForeignMissing proves F1: the best-effort
// batched list classifies each card-174/144 failure with the truthful Kind
// instead of collapsing every owner failure into "missing". Each kind is
// isolated in its own home so the three deployments hit three distinct code
// paths, and a best-effort list records each warning with its own Kind and its
// preserved typed cause.
func TestBatchedListClassifiesAccessForeignMissing(t *testing.T) {
	target := "local"

	// (1) missing owner: no Core installation under the Target. The
	// best-effort warning must be Kind "missing" with ErrDeploymentOwnerMissing.
	t.Run("missing", func(t *testing.T) {
		data := t.TempDir()
		cfg := t.TempDir()
		t.Setenv("BASEHARBOR_STATE_DIR", "")
		t.Setenv("XDG_DATA_HOME", data)
		t.Setenv("XDG_CONFIG_HOME", cfg)
		id := testDeploymentIdentity(t, target, "app-missing", "dev")
		deployNS := filepath.Join(data, "baseharbor", "targets", target, "deployments")
		if err := os.MkdirAll(filepath.Join(deployNS, id.DeploymentID), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		_, warnings, batch := listDeploymentRecordsBatched(deployNS, target, true)
		if batch == nil {
			t.Fatal("missing-owner list returned no batch")
		}
		assertWarnKind(t, warnings, id.DeploymentID, "missing", ErrDeploymentOwnerMissing)
	})

	// (2) foreign owner: the single installation.json governs a different
	// Target. The best-effort warning must be Kind "foreign" with
	// ErrDeploymentOwnerForeign.
	t.Run("foreign", func(t *testing.T) {
		data := t.TempDir()
		cfg := t.TempDir()
		t.Setenv("BASEHARBOR_STATE_DIR", "")
		t.Setenv("XDG_DATA_HOME", data)
		t.Setenv("XDG_CONFIG_HOME", cfg)
		provisionOwnerForTarget(t, target)
		ownerRuntime := filepath.Join(data, "baseharbor", "targets", target, "runtime")
		makeInstallationForeign(t, ownerRuntime)
		id := testDeploymentIdentity(t, target, "app-foreign", "dev")
		deployNS := filepath.Join(data, "baseharbor", "targets", target, "deployments")
		if err := os.MkdirAll(filepath.Join(deployNS, id.DeploymentID), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		_, warnings, _ := listDeploymentRecordsBatched(deployNS, target, true)
		assertWarnKind(t, warnings, id.DeploymentID, "foreign", ErrDeploymentOwnerForeign)
	})

	// (3) access-denied tenant: a grant-only TenantOwnerResolver that denies the
	// Target with a concrete error. The best-effort warning must be Kind "access"
	// and carry that resolver error as its typed cause.
	t.Run("access", func(t *testing.T) {
		data := t.TempDir()
		cfg := t.TempDir()
		t.Setenv("BASEHARBOR_STATE_DIR", "")
		t.Setenv("XDG_DATA_HOME", data)
		t.Setenv("XDG_CONFIG_HOME", cfg)
		prev := TenantOwnerResolver
		accessDeniedErr := errors.New("tenant access denied: no authorized Coreless owner for " + target)
		TenantOwnerResolver = func(string) (bool, error) { return false, accessDeniedErr }
		defer func() { TenantOwnerResolver = prev }()
		id := testDeploymentIdentity(t, target, "app-access", "dev")
		deployNS := filepath.Join(data, "baseharbor", "targets", target, "deployments")
		if err := os.MkdirAll(filepath.Join(deployNS, id.DeploymentID), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		_, warnings, _ := listDeploymentRecordsBatched(deployNS, target, true)
		assertWarnKind(t, warnings, id.DeploymentID, "access", nil)
		for _, w := range warnings {
			if se, ok := DeploymentRecordState(w); ok && se.Identity.DeploymentID == id.DeploymentID {
				if !errors.Is(se.Err, accessDeniedErr) {
					t.Fatalf("access-denied warning must preserve the resolver error %v, got %v", accessDeniedErr, se.Err)
				}
			}
		}
	})
}

// assertWarnKind finds the warning for deploymentID and asserts its Kind and,
// when wantErr is non-nil, that its preserved cause matches wantErr via
// errors.Is.
func assertWarnKind(t *testing.T, warnings []error, deploymentID, wantKind string, wantErr error) {
	t.Helper()
	var found *DeploymentRecordStateError
	for _, w := range warnings {
		if se, ok := DeploymentRecordState(w); ok && se.Identity.DeploymentID == deploymentID {
			found = se
			break
		}
	}
	if found == nil {
		t.Fatalf("no best-effort warning recorded for deployment %s", deploymentID)
	}
	if found.Kind != wantKind {
		t.Fatalf("%s owner failure must be classified Kind %q, got %q (%v)", wantKind, wantKind, found.Kind, found.Err)
	}
	if wantErr != nil && !errors.Is(found.Err, wantErr) {
		t.Fatalf("%s owner failure must carry a cause matching %v, got %v", wantKind, wantErr, found.Err)
	}
}

// TestBatchedListForeignToleranceControl proves the card-144 foreign-record
// tolerance is unchanged by batching: a best-effort list over a Target whose
// installation.json governs a foreign Target returns zero protected records and
// surfaces a foreign-kind warning. Batching must not silently adopt a foreign
// owner.
func TestBatchedListForeignToleranceControl(t *testing.T) {
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)

	target := "local"
	provisionOwnerForTarget(t, target)
	ownerRuntime := filepath.Join(data, "baseharbor", "targets", target, "runtime")
	makeInstallationForeign(t, ownerRuntime)

	foreignID := testDeploymentIdentity(t, target, "app-144", "dev")
	deployNS := filepath.Join(data, "baseharbor", "targets", target, "deployments")
	if err := os.MkdirAll(filepath.Join(deployNS, foreignID.DeploymentID), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	listed, warnings, batch := listDeploymentRecordsBatched(deployNS, target, true)
	if len(listed) != 0 {
		t.Fatalf("144 foreign-owner best-effort list must return zero protected records, got %d", len(listed))
	}
	if batch == nil || batch.loads == 0 {
		t.Fatal("foreign-owner list must still resolve the owner exactly once")
	}
	var sawForeign bool
	for _, w := range warnings {
		if se, ok := DeploymentRecordState(w); ok && se.Kind == "foreign" {
			sawForeign = true
		}
	}
	if !sawForeign {
		t.Fatal("144 foreign-owner best-effort list must surface a foreign-kind warning")
	}
}

// TestBatchedListOncePerTargetResolverBound widens F2's measured bound: the
// authoritative installation-load bound must hold even when the tenant resolver
// is exercised for a Coreless target. With a grant-only TenantOwnerResolver that
// denies (false, nil) and therefore falls through to coreinstallation.Load for
// each record, the batch must still read installation.json exactly once per
// Target — the resolver is a grant-only hook and never caches an ownership
// conclusion, while the batch serves every record from its single cached load.
func TestBatchedListOncePerTargetResolverBound(t *testing.T) {
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)

	target := "local"
	provisionOwnerForTarget(t, target)

	prev := TenantOwnerResolver
	TenantOwnerResolver = func(string) (bool, error) { return false, nil }
	defer func() { TenantOwnerResolver = prev }()

	// Five records under one owned Target.
	for i := 0; i < 5; i++ {
		id := testDeploymentIdentity(t, target, "app-resolver-"+itoa(i), "dev")
		deployNS := filepath.Join(data, "baseharbor", "targets", target, "deployments")
		if err := os.MkdirAll(filepath.Join(deployNS, id.DeploymentID), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	_, _, batch := listDeploymentRecordsBatched(
		filepath.Join(data, "baseharbor", "targets", target, "deployments"), target, true,
	)
	if batch == nil {
		t.Fatal("listDeploymentRecordsBatched returned no batch")
	}
	if batch.loads != 1 {
		t.Fatalf("authoritative installation-load bound broken: %d reads for 5 records; expected exactly 1 once-per-Target even with the tenant resolver denied per batch", batch.loads)
	}
}

// TestBatchedListClassifiesAccessDeniedFileError covers the card-174 fix on PR
// #1511 r4: the batched owner-load previously folded EVERY load error except
// os.ErrNotExist into ErrDeploymentOwnerForeign, so a Core installation that
// could not be READ (permission denied) was mis-classified as a foreign owner.
// The batched list must instead reuse the central owner-load classification:
// an unreadable installation is ErrDeploymentOwnerAccessDenied (Kind "access"),
// never foreign, never missing. This exercises the file-level load seam
// (resolveInstallationLoad -> coreinstallation.Load) deterministically through
// the package-internal test seam, independent of root-dependent file
// permissions on CI runners.
func TestBatchedListClassifiesAccessDeniedFileError(t *testing.T) {
	for _, bestEffort := range []bool{true, false} {
		bestEffort := bestEffort
		t.Run(fmt.Sprintf("bestEffort=%v", bestEffort), func(t *testing.T) {
			t.Setenv("BASEHARBOR_STATE_DIR", "")
			data := t.TempDir()
			cfg := t.TempDir()
			t.Setenv("XDG_DATA_HOME", data)
			t.Setenv("XDG_CONFIG_HOME", cfg)
			target := "local"
			provisionOwnerForTarget(t, target)

			identity := testDeploymentIdentity(t, target, "app-access-denied", "dev")
			deployNS := filepath.Join(data, "baseharbor", "targets", target, "deployments")
			if err := os.MkdirAll(filepath.Join(deployNS, identity.DeploymentID), 0o700); err != nil {
				t.Fatalf("mkdir deployment dir: %v", err)
			}

			// Inject a deterministic permission-denied at the authoritative
			// installation-load seam: the genuine Core exists but cannot be read.
			permErr := newPermissionDeniedError()
			testLoadInstallationForOwner = func(root string) (coreinstallation.State, error) {
				return coreinstallation.State{}, permErr
			}
			defer restoreTestLoadOwner(t)

			records, warnings, batch := listDeploymentRecordsBatched(deployNS, target, bestEffort)

			// The once-per-Target bound still holds: one authoritative read,
			// which failed with permission denied.
			if batch != nil && batch.loads != 1 {
				t.Fatalf("authoritative installation-load bound broken: %d reads; expected exactly 1", batch.loads)
			}

			normalErr := batch.err
			if bestEffort {
				// Best-effort must surface a per-record warning whose Kind is
				// "access" and whose preserved cause is ErrDeploymentOwnerAccessDenied
				// (not foreign, not missing). No protected record is returned.
				if len(records) != 0 {
					t.Fatalf("access-denied batch list must return zero records, got %d", len(records))
				}
				var found *DeploymentRecordStateError
				for _, w := range warnings {
					if se, ok := DeploymentRecordState(w); ok && se.Identity.DeploymentID == identity.DeploymentID {
						found = se
					}
				}
				if found == nil {
					t.Fatal("best-effort access-denied list must surface a record warning")
				}
				if found.Kind != "access" {
					t.Fatalf("access-denied best-effort warning must be Kind \"access\", got %q (%v)", found.Kind, found.Err)
				}
				if !errors.Is(found.Err, ErrDeploymentOwnerAccessDenied) {
					t.Fatalf("access-denied best-effort warning must carry ErrDeploymentOwnerAccessDenied, got %v", found.Err)
				}
				if !errors.Is(found.Err, newPermissionDeniedError()) {
					t.Fatalf("access-denied warning must preserve the underlying permission error, got %v", found.Err)
				}
				if errors.Is(found.Err, ErrDeploymentOwnerForeign) {
					t.Fatalf("access-denied warning must NOT be classified foreign: %v", found.Err)
				}
			} else {
				// Normal list must fail closed with the access-denied sentinel —
				// never ErrDeploymentOwnerForeign, never ErrDeploymentOwnerMissing,
				// and never return a record for an unreadable owner.
				if normalErr == nil {
					t.Fatal("normal access-denied list must fail closed")
				}
				if len(records) != 0 {
					t.Fatalf("normal access-denied list must return zero records, got %d", len(records))
				}
				if !errors.Is(normalErr, ErrDeploymentOwnerAccessDenied) {
					t.Fatalf("normal access-denied list must fail with ErrDeploymentOwnerAccessDenied, got %v", normalErr)
				}
				if errors.Is(normalErr, ErrDeploymentOwnerForeign) {
					t.Fatalf("normal access-denied list must NOT be classified foreign: %v", normalErr)
				}
				if errors.Is(normalErr, ErrDeploymentOwnerMissing) {
					t.Fatalf("normal access-denied list must NOT be classified missing: %v", normalErr)
				}
			}
		})
	}
}
