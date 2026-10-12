package traces

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestRetainedTempoInventoryPreservesDataOwnerAndExternalDependencies(t *testing.T) {
	for _, ha := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "ha"}[ha], func(t *testing.T) {
			root := t.TempDir()
			issuer := serviceissuer.New(t)
			m := application.WithAvailabilityOverride(application.New("retained-traces", "dev", false, false, false), "traces", &ha, 0)
			var storage *objectstorage.PlatformBucket
			if ha {
				objectFiles, err := objectstorage.EnsureProviderFilesAt(t.Context(), issuer, root, "isolated")
				if err != nil {
					t.Fatal(err)
				}
				storage = &objectstorage.PlatformBucket{Network: objectFiles.Network, TrustBundle: filepath.Join(objectFiles.Dir, "service-access", "runtime", "ca.pem")}
			}
			files, p, err := ensureProviderFilesAt(t.Context(), issuer, root, "isolated", m, storage)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(files.Env)
			placement := capability.ProviderPlacement{Scope: p.Scope, Ownership: capability.OwnershipBaseHarbor}
			_, _, resources, err := ExistingRuntimeInventoryAt(root, "isolated", m, placement)
			if err != nil || len(resources) == 0 {
				t.Fatalf("retained canonical Tempo owner not observed: %v", err)
			}
			after, _ := os.ReadFile(files.Env)
			if !bytes.Equal(before, after) {
				t.Fatal("read-only inventory changed retained access material")
			}
			if storage != nil {
				for _, r := range resources {
					if r.Kind == "network" && r.Name == storage.Network {
						t.Fatal("external storage dependency adopted for retirement")
					}
				}
			}
			placement.Ownership = capability.OwnershipExternal
			if _, _, _, err := ExistingRuntimeInventoryAt(root, "isolated", m, placement); err == nil {
				t.Fatal("BYO ownership granted retirement authority")
			}
		})
	}
}
