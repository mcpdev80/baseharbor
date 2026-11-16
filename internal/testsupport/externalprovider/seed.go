package externalprovider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
)

// SeedProviderRegistryOwner provisions an owned, non-group-writable provider
// registry for a test target so the external-provider removal ownership seam
// (verifyExternalRemovalRegistryOwner in internal/application) can verify
// ownership from the fixture alone, independent of the ambient tmp on the CI
// runner.
//
// This is the internal/testsupport fixture equivalent of the cmd/baha removal
// test seed (PR #1351, work/r27-envdep-ab): the check treats a group-writable
// ancestor of the registry as "provider-registry-ownership-unverifiable", so the
// seeded state dir must be owned and non-group-writable. The helper makes the
// state dir mode 0o700 (os.Chmod(root, 0o700)) and registers a real
// external provider + a bound target so the seed is genuine owner-material
// through the production registration lifecycle — NOT a global bypass, NOT an
// off-switch, NOT a skip: the ownership seam still runs.
//
// Callers invoke SeedProviderRegistryOwner at the top of a removal test body,
// AFTER t.Setenv for BASEHARBOR_STATE_DIR / XDG_* (so the state dir lands in
// that test's temp home) and BEFORE any t.Chdir that would change where the
// registry is written. It returns the seeded root so callers can load or assert
// against the resulting provider-registry.json.
func SeedProviderRegistryOwner(t *testing.T, target string) (root string) {
	t.Helper()

	root = t.TempDir()
	// Seed ownership explicitly so the provider-registry ownership check can be
	// verified from the fixture alone (no reliance on a non-group-writable tmp
	// on the CI runner). The verification treats a group-writable ancestor as
	// unverifiable ownership, so the seeded state dir must be owned and
	// non-group-writable.
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatalf("chmod seeded state dir %s to 0o700: %v", root, err)
	}
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	reg := externalprovider.Registration{
		ID:         "team-db",
		ProviderID: "team/postgresql",
		Provider:   capability.Provider{Kind: capability.ProviderPostgreSQL, Capabilities: []capability.Kind{capability.SQL}},
		Endpoint:   "postgres://db.example:5432/app",
		Trust:      externalprovider.Trust{Mode: externalprovider.TrustSystem},
	}
	if err := application.RegisterExternalProviderAt(root, reg); err != nil {
		t.Fatalf("register external provider at seeded root: %v", err)
	}

	r, err := (capability.RegistryStore{Path: filepath.Join(root, "provider-registry.json")}).Load()
	if err != nil {
		t.Fatalf("load seeded provider registry: %v", err)
	}
	store := capability.RegistryStore{Path: filepath.Join(root, "targets", target, "provider-registry.json")}
	if err := store.Update(func(tgt *capability.Registry) error {
		if err := tgt.Register(r.Instances[0]); err != nil {
			return err
		}
		return tgt.BindDeployment(
			capability.Resource{Application: "web", Kind: capability.SQL, Name: "default", Provider: capability.ProviderPostgreSQL},
			application.MustNewApplicationID(),
			"dev",
			r.Instances[0].ID,
		)
	}); err != nil {
		t.Fatalf("bind provider to target %s: %v", target, err)
	}

	return root
}
