package logs

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestRetainedLokiInventoryPreservesOwnersAndExternalStorage(t *testing.T) {
	for _, scope := range []capability.ProviderScope{capability.ScopeShared, capability.ScopeApplication} {
		for _, mode := range []bhruntime.LogCollectionMode{bhruntime.LogCollectionSyslog, bhruntime.LogCollectionJournald} {
			for _, ha := range []bool{false, true} {
				t.Run(string(scope)+"/"+string(mode)+map[bool]string{false: "/single", true: "/ha"}[ha], func(t *testing.T) {
					root := t.TempDir()
					t.Setenv(application.ProviderScopeEnv(capability.ProviderLoki), string(scope))
					issuer := serviceissuer.New(t)
					m := application.WithAvailabilityOverride(application.New("retained-logs", "dev", false, false, false), "logs", &ha, 0)
					var storage *objectstorage.PlatformBucket
					if ha {
						f, err := objectstorage.EnsureProviderFilesAt(t.Context(), issuer, root, "isolated")
						if err != nil {
							t.Fatal(err)
						}
						storage = &objectstorage.PlatformBucket{Network: f.Network, TrustBundle: filepath.Join(f.Dir, "service-access", "runtime", "ca.pem")}
					}
					files, err := ensureProviderFilesForModeAt(t.Context(), issuer, root, "isolated", m, mode, storage)
					if err != nil {
						t.Fatal(err)
					}
					before, _ := os.ReadFile(files.Env)
					placement := capability.ProviderPlacement{Scope: scope, Ownership: capability.OwnershipBaseHarbor}
					_, _, resources, err := ExistingRuntimeInventoryAt(root, "isolated", m, placement, mode)
					if err != nil || len(resources) == 0 {
						t.Fatalf("canonical owner: %v", err)
					}
					after, _ := os.ReadFile(files.Env)
					if !bytes.Equal(before, after) {
						t.Fatal("observation changed access material")
					}
					if storage != nil {
						for _, resource := range resources {
							if resource.Kind == "network" && resource.Name == storage.Network {
								t.Fatal("external storage adopted")
							}
						}
					}
					placement.Ownership = capability.OwnershipExternal
					if _, _, _, err := ExistingRuntimeInventoryAt(root, "isolated", m, placement, mode); err == nil {
						t.Fatal("foreign owner accepted")
					}
					placement.Ownership = capability.OwnershipBaseHarbor
					data, _ := os.ReadFile(files.Compose)
					if err := os.WriteFile(files.Compose, append(data, []byte("\n# changed owner\n")...), 0600); err != nil {
						t.Fatal(err)
					}
					if _, _, _, err := ExistingRuntimeInventoryAt(root, "isolated", m, placement, mode); err == nil {
						t.Fatal("changed definition accepted")
					}
				})
			}
		}
	}
}
