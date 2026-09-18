package repositoryinspect

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectCollectsDeterministicRepositoryEvidence(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "compose.yaml", `services:
  api:
    build: .
    ports:
      - "8080:8080"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
  database:
    image: postgres:17
  cache:
    image: valkey/valkey:8
`)
	writeTestFile(t, root, ".env.example", "DATABASE_URL=\nREDIS_URL=\nOPENAI_API_KEY=\nPUBLIC_URL=\n")
	writeTestFile(t, root, "package.json", `{"dependencies":{"pg":"latest","ioredis":"latest"}}`)
	writeTestFile(t, root, "Dockerfile", "FROM scratch\n")

	result, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}

	if result.SelectedCompose != "compose.yaml" {
		t.Fatalf("SelectedCompose = %q", result.SelectedCompose)
	}
	if len(result.WorkloadServices) != 1 || result.WorkloadServices[0] != "api" {
		t.Fatalf("WorkloadServices = %#v", result.WorkloadServices)
	}
	assertFindingConfidence(t, result, "database.sql", ConfidenceDetected)
	assertFindingConfidence(t, result, "cache.key-value", ConfidenceDetected)
	if len(result.SecretCandidates) != 1 || result.SecretCandidates[0] != "OPENAI_API_KEY" {
		t.Fatalf("SecretCandidates = %#v", result.SecretCandidates)
	}
	if len(result.Ports) != 1 || result.Ports[0].Service != "api" || result.Ports[0].Value != "8080:8080" {
		t.Fatalf("Ports = %#v", result.Ports)
	}
	if len(result.HealthChecks) != 1 {
		t.Fatalf("HealthChecks = %#v", result.HealthChecks)
	}
	if _, err := os.Stat(filepath.Join(root, "baseharbor.yaml")); !os.IsNotExist(err) {
		t.Fatalf("inspection mutated repository, stat err = %v", err)
	}
}

func TestInspectDistinguishesSuggestedFromDetected(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module example.test/app\nrequire github.com/jackc/pgx/v5 v5.0.0\n")

	result, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	assertFindingConfidence(t, result, "database.sql", ConfidenceSuggested)
}

func TestInspectReportsPossibleEndpointConfiguration(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "config.yaml", "backend: postgresql://db.internal/app\n")

	result, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	assertFindingConfidence(t, result, "database.sql", ConfidencePossible)
}

func TestInspectDoesNotGuessBetweenMultipleComposeFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "compose.yaml", "services:\n  api:\n    image: example/api\n")
	writeTestFile(t, root, "deploy/docker-compose.yml", "services:\n  worker:\n    image: example/worker\n")

	result, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ComposeCandidates) != 2 {
		t.Fatalf("ComposeCandidates = %#v", result.ComposeCandidates)
	}
	if result.SelectedCompose != "" {
		t.Fatalf("SelectedCompose = %q, want empty on ambiguity", result.SelectedCompose)
	}
	if len(result.WorkloadServices) != 0 {
		t.Fatalf("WorkloadServices = %#v, want none on ambiguity", result.WorkloadServices)
	}
}

func TestEngineCanRegisterAdditionalDetector(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "marker.conf", "enabled=true\n")
	engine := DefaultEngine()
	engine.Detectors = append(engine.Detectors, staticDetector{})

	result, err := engine.Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	assertFindingConfidence(t, result, "example.custom", ConfidenceDetected)
}

func TestMarshalJSONResultIsMachineReadable(t *testing.T) {
	result := Result{
		Root:        "/repo",
		Application: "demo",
		Findings: []Finding{{
			Capability: "database.sql",
			Confidence: ConfidenceDetected,
			Evidence:   []Evidence{{Kind: EvidenceEnv, Path: ".env.example", Detail: "variable DATABASE_URL"}},
		}},
	}
	data, err := MarshalJSONResult(result)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"application": "demo"`, `"confidence": "detected"`, `"capability": "database.sql"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("JSON missing %s: %s", want, text)
		}
	}
}

func TestParsePublishedPort(t *testing.T) {
	for _, test := range []struct {
		value string
		want  int
		ok    bool
	}{
		{"8080:80", 8080, true},
		{"127.0.0.1:8443:443", 8443, true},
		{"80", 0, false},
		{"8000-8010:80", 0, false},
	} {
		got, ok := ParsePublishedPort(test.value)
		if got != test.want || ok != test.ok {
			t.Fatalf("ParsePublishedPort(%q) = %d,%v want %d,%v", test.value, got, ok, test.want, test.ok)
		}
	}
}

type staticDetector struct{}

func (staticDetector) Name() string { return "example.custom" }

func (staticDetector) Detect(context.Context, Snapshot) ([]Finding, error) {
	return []Finding{{
		Capability: "example.custom",
		Confidence: ConfidenceDetected,
		Evidence:   []Evidence{{Kind: EvidenceConfig, Path: "marker.conf", Detail: "test marker"}},
	}}, nil
}

func assertFindingConfidence(t *testing.T, result Result, capability string, want Confidence) {
	t.Helper()
	for _, finding := range result.Findings {
		if finding.Capability == capability {
			if finding.Confidence != want {
				t.Fatalf("%s confidence = %q, want %q", capability, finding.Confidence, want)
			}
			if len(finding.Evidence) == 0 {
				t.Fatalf("%s has no evidence", capability)
			}
			return
		}
	}
	t.Fatalf("finding %s not present: %#v", capability, result.Findings)
}

func writeTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInspectDoesNotExposeEnvValues(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, ".env", "DATABASE_URL=postgresql://user:super-secret@db/app\nAPI_TOKEN=very-secret\n")

	result, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalJSONResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "super-secret") || strings.Contains(string(data), "very-secret") {
		t.Fatalf("inspection leaked env value: %s", data)
	}
}

func TestInspectSkipsSymlinkedFiles(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.conf")
	if err := os.WriteFile(outside, []byte("postgresql://outside/secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked.conf")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	result, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range result.Findings {
		for _, evidence := range finding.Evidence {
			if evidence.Path == "linked.conf" {
				t.Fatalf("symlink evidence should be ignored: %#v", finding)
			}
		}
	}
}

func TestInspectDoesNotTreatApplicationEnvAsProviderService(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "compose.yaml", `services:
  api:
    image: example/api
    environment:
      DATABASE_URL: postgresql://database/app
  database:
    image: postgres:17
`)

	result, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.WorkloadServices) != 1 || result.WorkloadServices[0] != "api" {
		t.Fatalf("WorkloadServices = %#v", result.WorkloadServices)
	}
	for _, finding := range result.Findings {
		if finding.Capability == "database.sql" && finding.Name == "api" {
			t.Fatalf("application workload misclassified as database provider: %#v", finding)
		}
	}
}

func TestInspectCollectsDockerfilePortsAndHealthcheck(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "Dockerfile", "FROM scratch\nEXPOSE 8080 8443/tcp\nHEALTHCHECK CMD true\n")

	result, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Ports) != 2 {
		t.Fatalf("Ports = %#v", result.Ports)
	}
	if len(result.HealthChecks) != 1 || result.HealthChecks[0].Path != "Dockerfile" {
		t.Fatalf("HealthChecks = %#v", result.HealthChecks)
	}
}
