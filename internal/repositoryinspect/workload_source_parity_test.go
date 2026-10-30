package repositoryinspect

import (
	"context"
	"path/filepath"
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

// TestInspectProjectsNeutralSummaryForAllSourceFamilies asserts that the shared
// inspection summary (ports, health/readiness, workload vs infrastructure
// classification and capability findings) is projected from normalized workload
// evidence for every supported source family, not only Compose. This is the
// core-750 contract: Quadlet and Kubernetes repositories must not under-report.
func TestInspectProjectsNeutralSummaryForAllSourceFamilies(t *testing.T) {
	fixtures := map[string]struct {
		composePath    string
		paths          []string
		components     []string
		infrastructure map[string]string
	}{
		"compose": {
			composePath:    "compose.yaml",
			components:     []string{"api", "worker", "db", "cache"},
			infrastructure: map[string]string{"db": "database.sql", "cache": "cache.key-value"},
		},
		"quadlet": {
			paths:          []string{"api.container", "worker.container", "db.container", "cache.container"},
			components:     []string{"api", "worker", "db", "cache"},
			infrastructure: map[string]string{"db": "database.sql", "cache": "cache.key-value"},
		},
		"kubernetes": {
			paths:          []string{"app.yaml"},
			components:     []string{"api", "worker", "db", "cache"},
			infrastructure: map[string]string{"db": "database.sql", "cache": "cache.key-value"},
		},
	}

	for name, fx := range fixtures {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join("..", "..", "testdata", "adoption-parity", name)
			result, err := Inspect(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}

			for _, id := range fx.components {
				if !contains(result.WorkloadServices, id) && !contains(result.InfrastructureServices, id) {
					t.Fatalf("%s: component %q not projected to workload/infrastructure services: workload=%v infra=%v",
						name, id, result.WorkloadServices, result.InfrastructureServices)
				}
			}
			if len(result.Ports) == 0 {
				t.Fatalf("%s: ports not projected from normalized component evidence", name)
			}
			if len(result.HealthChecks) == 0 {
				t.Fatalf("%s: health/readiness not projected from normalized component evidence", name)
			}
			for id, class := range fx.infrastructure {
				if !contains(result.InfrastructureServices, id) {
					t.Fatalf("%s: infrastructure component %q (%s) not projected: infra=%v", name, id, class, result.InfrastructureServices)
				}
				assertFindingConfidence(t, result, class, ConfidenceDetected)
			}
			logFinding := resultFindByCapability(result, "logs")
			if logFinding == nil {
				t.Fatal("logs suggestion not projected; it must not be Compose-only")
			}
			assertSourceEvidenceKind(t, name, *logFinding)
		})
	}
}

// assertSourceEvidenceKind verifies the workload log suggestion carries the
// evidence kind of the selected source family: Compose records compose, every
// other family records config provenance and never falsely claims Compose.
func assertSourceEvidenceKind(t *testing.T, name string, finding Finding) {
	t.Helper()
	if len(finding.Evidence) == 0 {
		t.Fatalf("%s: logs finding has no evidence", name)
	}
	if name == "compose" && finding.Evidence[0].Kind != EvidenceCompose {
		t.Fatalf("%s: compose logs finding kind = %q, want compose", name, finding.Evidence[0].Kind)
	}
	if name != "compose" && finding.Evidence[0].Kind == EvidenceCompose {
		t.Fatalf("%s: non-compose logs finding must not claim compose evidence, got %q", name, finding.Evidence[0].Kind)
	}
	if name != "compose" && finding.Evidence[0].Kind != EvidenceConfig {
		t.Fatalf("%s: non-compose logs finding kind = %q, want config", name, finding.Evidence[0].Kind)
	}
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func resultFindByName(result Result, name string) *Finding {
	for i := range result.Findings {
		if result.Findings[i].Name == name {
			return &result.Findings[i]
		}
	}
	return nil
}

func resultFindByCapability(result Result, capability string) *Finding {
	for i := range result.Findings {
		if result.Findings[i].Capability == capability {
			return &result.Findings[i]
		}
	}
	return nil
}
