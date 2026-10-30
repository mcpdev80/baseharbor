package repositoryinspect

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkloadSourceCrossSourceSemanticParity(t *testing.T) {
	fixtures := map[string]string{
		"compose":    filepath.Join("..", "..", "testdata", "adoption-parity", "compose"),
		"quadlet":    filepath.Join("..", "..", "testdata", "adoption-parity", "quadlet"),
		"kubernetes": filepath.Join("..", "..", "testdata", "adoption-parity", "kubernetes"),
	}

	for name, root := range fixtures {
		t.Run(name, func(t *testing.T) {
			snapshot, _, err := collectSnapshot(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}
			candidates, err := InspectWorkloadSources(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if len(candidates) != 1 {
				t.Fatalf("candidates = %#v", candidates)
			}
			evidence, err := NormalizeWorkloadSource(snapshot, candidates[0])
			if err != nil {
				t.Fatal(err)
			}
			byID := map[string]WorkloadComponent{}
			for _, component := range evidence.Components {
				byID[component.ID] = component
				if len(component.Source) == 0 {
					t.Fatalf("%s has no provenance: %#v", component.ID, component)
				}
			}
			for _, id := range []string{"api", "worker", "db", "cache"} {
				if _, ok := byID[id]; !ok {
					t.Fatalf("missing logical component %q: %#v", id, evidence.Components)
				}
			}
			if byID["db"].InfrastructureClass != "database.sql" {
				t.Fatalf("db classification = %#v", byID["db"])
			}
			if byID["cache"].InfrastructureClass != "cache.key-value" {
				t.Fatalf("cache classification = %#v", byID["cache"])
			}
			if len(byID["api"].Ports) == 0 {
				t.Fatalf("api port evidence missing: %#v", byID["api"])
			}
			if len(byID["api"].Exposure) == 0 {
				t.Fatalf("api exposure evidence missing: %#v", byID["api"])
			}
			if !byID["api"].Health {
				t.Fatalf("api health/readiness evidence missing: %#v", byID["api"])
			}
			if evidence.Fingerprint == "" {
				t.Fatal("fingerprint missing")
			}
		})
	}
}

func TestInspectProjectsNeutralSummaryForAllSourceFamilies(t *testing.T) {
	fixtures := map[string]struct {
		expectWorkload   []string
		expectInfra      []string
		wantPorts        int
		wantHealthChecks int
	}{
		"compose":    {expectWorkload: []string{"api", "worker"}, expectInfra: []string{"cache", "db"}, wantPorts: 1, wantHealthChecks: 1},
		"quadlet":    {expectWorkload: []string{"api", "worker"}, expectInfra: []string{"cache", "db"}, wantPorts: 1, wantHealthChecks: 1},
		"kubernetes": {expectWorkload: []string{"api", "worker"}, expectInfra: []string{"cache", "db"}, wantPorts: 1, wantHealthChecks: 1},
	}

	for name, fx := range fixtures {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join("..", "..", "testdata", "adoption-parity", name)
			result, err := Inspect(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}

			if strings.Join(result.WorkloadServices, ",") != strings.Join(fx.expectWorkload, ",") {
				t.Fatalf("workload_services = %#v, want %#v", result.WorkloadServices, fx.expectWorkload)
			}
			if strings.Join(result.InfrastructureServices, ",") != strings.Join(fx.expectInfra, ",") {
				t.Fatalf("infrastructure_services = %#v, want %#v", result.InfrastructureServices, fx.expectInfra)
			}
			if len(result.Ports) < fx.wantPorts {
				t.Fatalf("ports = %d, want at least %d", len(result.Ports), fx.wantPorts)
			}
			if len(result.HealthChecks) < fx.wantHealthChecks {
				t.Fatalf("health_checks = %d, want at least %d", len(result.HealthChecks), fx.wantHealthChecks)
			}
			if !hasFindingCap(result, "logs") {
				t.Fatalf("expected workload log suggestion for %s: %#v", name, result.Findings)
			}
			if len(result.WorkloadEvidence.Components) == 0 {
				t.Fatal("no normalized components")
			}
		})
	}
}

func joinList(values []string) string {
	return strings.Join(values, ",")
}

func hasFindingCap(result Result, capability string) bool {
	for _, finding := range result.Findings {
		if finding.Capability == capability {
			return true
		}
	}
	return false
}
