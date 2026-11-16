package metrics

import (
	"bytes"
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestRetainedPrometheusInventoryPreservesCredentialsAndConsumerNetworks(t *testing.T) {
	for _, scope := range []capability.ProviderScope{capability.ScopeShared, capability.ScopeApplication} {
		for _, ha := range []bool{false, true} {
			t.Run(string(scope)+map[bool]string{false: "/single", true: "/ha"}[ha], func(t *testing.T) {
				root := t.TempDir()
				t.Setenv(application.ProviderScopeEnv(capability.ProviderPrometheus), string(scope))
				m := application.WithAvailabilityOverride(application.New("retained-metrics", "dev", false, false, false), "metrics", &ha, 0)
				files, err := EnsureProviderFilesWithRuntimeCAAt(t.Context(), serviceissuer.New(t), root, "isolated", m, "")
				if err != nil {
					t.Fatal(err)
				}
				before, _ := os.ReadFile(files.Env)
				placement := capability.ProviderPlacement{Scope: scope, Ownership: capability.OwnershipBaseHarbor}
				_, _, resources, err := ExistingRuntimeInventoryAt(root, "isolated", m, placement)
				if err != nil || len(resources) == 0 {
					t.Fatalf("canonical inventory: %v", err)
				}
				after, _ := os.ReadFile(files.Env)
				if !bytes.Equal(before, after) {
					t.Fatal("observation changed credentials")
				}
				for _, resource := range resources {
					if resource.Kind == "network" && resource.Name == registrationForAt(m, "isolated").Network {
						t.Fatal("consumer network adopted as exclusive provider owner")
					}
				}
				placement.Ownership = capability.OwnershipExternal
				if _, _, _, err := ExistingRuntimeInventoryAt(root, "isolated", m, placement); err == nil {
					t.Fatal("foreign owner accepted")
				}
				placement.Ownership = capability.OwnershipBaseHarbor
				data, _ := os.ReadFile(files.Compose)
				if err := os.WriteFile(files.Compose, append(data, []byte("\n# changed owner\n")...), 0600); err != nil {
					t.Fatal(err)
				}
				if _, _, _, err := ExistingRuntimeInventoryAt(root, "isolated", m, placement); err == nil {
					t.Fatal("changed definition accepted")
				}
			})
		}
	}
}
