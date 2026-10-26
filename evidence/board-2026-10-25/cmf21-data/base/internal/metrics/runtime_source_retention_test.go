package metrics

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestRetainedPrometheusInventorySurvivesOtherProviderSourceWithdrawal(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPrometheus), "shared")
	root := t.TempDir()
	m := application.WithMetricsSource(application.New("retained-source", "dev", false, false, false), "api", "api", 8080, "/metrics")
	source := observability.MetricsSource{ID: "tempo:retained", Provider: capability.ProviderTempo, Class: observability.SourcePlatformProvider, Scope: capability.ScopeShared, Network: "independent-trace-network", Target: "tempo:3200", Path: "/metrics"}
	if err := observability.Update(source); err != nil {
		t.Fatal(err)
	}
	files, err := EnsureProviderFilesWithRuntimeCAAt(t.Context(), serviceissuer.New(t), root, "isolated", m, "")
	if err != nil {
		t.Fatal(err)
	}
	placement := capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
	_, _, before, err := ExistingRuntimeInventoryAt(root, "isolated", m, placement)
	if err != nil {
		t.Fatal("original canonical Prometheus owner failed before source withdrawal", err)
	}
	definition, _ := os.ReadFile(files.Compose)
	credentials, _ := os.ReadFile(files.Env)
	if err := observability.Remove(source.ID); err != nil {
		t.Fatal(err)
	}
	_, _, after, err := ExistingRuntimeInventoryAt(root, "isolated", m, placement)
	if err != nil || len(after) != len(before) {
		t.Fatalf("retained canonical Prometheus owner became unverifiable after an independent provider withdrew: %v", err)
	}
	currentDefinition, _ := os.ReadFile(files.Compose)
	currentCredentials, _ := os.ReadFile(files.Env)
	if !bytes.Equal(definition, currentDefinition) || !bytes.Equal(credentials, currentCredentials) {
		t.Fatal("read-only inventory changed original runtime or authentication material")
	}
	snapshotPath := filepath.Join(files.Dir, "runtime-sources.json")
	original, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"project", "scope", "duplicate-source", "permissions", "symlink", "missing"} {
		t.Run(invalid, func(t *testing.T) {
			if err := os.Remove(snapshotPath); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = os.Remove(snapshotPath)
				if err := os.WriteFile(snapshotPath, original, 0600); err != nil {
					t.Fatal(err)
				}
			})
			var retained runtimeSourceSnapshot
			if err := json.Unmarshal(original, &retained); err != nil {
				t.Fatal(err)
			}
			switch invalid {
			case "project":
				retained.Project = "foreign-project"
			case "scope":
				retained.Scope = capability.ScopeExternal
			case "duplicate-source":
				retained.Sources = append(retained.Sources, retained.Sources[0])
			}
			data, err := json.Marshal(retained)
			if err != nil {
				t.Fatal(err)
			}
			if invalid == "symlink" {
				foreign := filepath.Join(t.TempDir(), "foreign-source.json")
				if err := os.WriteFile(foreign, original, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(foreign, snapshotPath); err != nil {
					t.Fatal(err)
				}
			} else if invalid != "missing" {
				mode := os.FileMode(0600)
				if invalid == "permissions" {
					mode = 0644
				}
				if err := os.WriteFile(snapshotPath, data, mode); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, _, err := ExistingRuntimeInventoryAt(root, "isolated", m, placement); err == nil {
				t.Fatal("damaged retained authority or unverifiable legacy owner accepted")
			}
		})
	}
	if err := os.WriteFile(files.Compose, append(definition, []byte("\n# foreign definition\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ExistingRuntimeInventoryAt(root, "isolated", m, placement); err == nil {
		t.Fatal("altered runtime definition accepted after source withdrawal")
	}
}
