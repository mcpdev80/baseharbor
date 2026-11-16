package telemetry

import (
	"bytes"
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/availability"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestRetainedCollectorInventoryKeepsBindingAndRejectsForeignOwner(t *testing.T) {
	for _, ha := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "ha"}[ha], func(t *testing.T) {
			root := t.TempDir()
			files, err := EnsureProviderFilesWithTraceBackendForEnvironmentAt(t.Context(), serviceissuer.New(t), "http://tempo:4318", "selected-owned-traces", "prod", root, "isolated", availability.Requirement{HA: ha})
			if err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(files.Env)
			_, resources, err := ExistingRuntimeInventoryAt(root, "isolated", "selected-owned-traces")
			if err != nil || len(resources) == 0 {
				t.Fatalf("existing collector owner not observed: %v", err)
			}
			after, _ := os.ReadFile(files.Env)
			if !bytes.Equal(before, after) {
				t.Fatal("read-only inventory changed retained environment")
			}
			for _, resource := range resources {
				if resource.Kind == "network" && resource.Name == "selected-owned-traces" {
					t.Fatal("consumer network adopted by collector retirement")
				}
			}
			if _, _, err := ExistingRuntimeInventoryAt(root, "isolated", "foreign-network"); err == nil {
				t.Fatal("different consumer binding admitted as canonical owner")
			}
		})
	}
}
