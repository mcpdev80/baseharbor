package traces

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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
			if needed, err := ExistingRuntimeUpgradeRequiredAt(root, "isolated", m, placement); err != nil || needed {
				t.Fatalf("current canonical runtime invented a version change: %v", err)
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
			current, err := os.ReadFile(files.Compose)
			if err != nil {
				t.Fatal(err)
			}
			legacy := strings.ReplaceAll(string(current), "image: "+ProviderImage+"\n", "image: docker.io/grafana/tempo:3.0.2\n")
			if err := os.WriteFile(files.Compose, []byte(legacy), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, _, err := ExistingRuntimeInventoryAt(root, "isolated", m, placement); err != nil {
				t.Fatalf("previous exact canonical owner lost retirement authority: %v", err)
			}
			if needed, err := ExistingRuntimeUpgradeRequiredAt(root, "isolated", m, placement); err != nil || !needed {
				t.Fatalf("retained previous image was silently reused without a planned update: %v", err)
			}
			unchanged, _ := os.ReadFile(files.Compose)
			if string(unchanged) != legacy {
				t.Fatal("inventory implicitly upgraded retained runtime")
			}
			for _, changed := range []string{
				strings.ReplaceAll(legacy, "tempo:3.0.2", "tempo:arbitrary"),
				strings.ReplaceAll(legacy, "read_only: true", "read_only: false"),
				legacy + "# changed owner\n",
			} {
				if err := os.WriteFile(files.Compose, []byte(changed), 0o600); err != nil {
					t.Fatal(err)
				}
				if _, _, _, err := ExistingRuntimeInventoryAt(root, "isolated", m, placement); err == nil {
					t.Fatal("noncanonical previous owner granted authority")
				}
			}
			if err := os.WriteFile(files.Compose, []byte(legacy), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(files.Compose, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, _, _, err := ExistingRuntimeInventoryAt(root, "isolated", m, placement); err == nil {
				t.Fatal("unprotected previous owner granted authority")
			}
			if err := os.Chmod(files.Compose, 0o600); err != nil {
				t.Fatal(err)
			}
			placement.Ownership = capability.OwnershipExternal
			if _, _, _, err := ExistingRuntimeInventoryAt(root, "isolated", m, placement); err == nil {
				t.Fatal("BYO ownership granted retirement authority")
			}
		})
	}
}

func TestTempoShutdownFixKeepsExistingBlockWriteFormat(t *testing.T) {
	for _, config := range []string{configYAML(), tempoHAConfig()} {
		if !strings.Contains(config, "    block:\n      version: vParquet4\n") {
			t.Fatal("Tempo upgrade silently changes retained block write format")
		}
	}
}
