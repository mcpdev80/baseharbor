package main

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	repositoryinspect "github.com/mcpdev80/baseharbor/internal/repositoryinspect"
)

// Boundary regression: arbitrary candidate paths (health checks, API routes,
// prefix/substring variants) must NOT count as proven metrics endpoints. The
// detector keeps such repositories out of the "detected metrics" path and
// quick init must never bind such a path into the manifest.

func TestMetricsProvenEndpointIsExactRegistryOnly(t *testing.T) {
	proven := []string{"/q/metrics", "/actuator/prometheus", "/metrics"}
	for _, path := range proven {
		if !metricsProvenEndpoint(path) {
			t.Fatalf("%q must be a proven metrics endpoint", path)
		}
	}
	notProven := []string{
		"/health", "/healthz", "/api/users", "/metrics-old", "/old-metrics",
		"/myorg/metrics", "/actuator", "/q/metrics-v2", "/METRICS", "metrics", "",
	}
	for _, path := range notProven {
		if metricsProvenEndpoint(path) {
			t.Fatalf("%q must NOT be a proven metrics endpoint", path)
		}
	}
}

func TestMetricsProvenPathRejectsArbitraryCandidates(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   string
		wantOK bool
	}{
		{"quarkus detail", "api/application.properties repository exposes a metrics endpoint /q/metrics (quarkus)", "/q/metrics", true},
		{"spring detail", "api/application.properties repository exposes a metrics endpoint /actuator/prometheus (spring)", "/actuator/prometheus", true},
		{"conventional detail", "main.go repository exposes a metrics endpoint /metrics (conventional)", "/metrics", true},
		{"health only", "main.go repository exposes a metrics endpoint /health (other)", "", false},
		{"prefix substring", "main.go repository exposes a metrics endpoint /metrics-old (other)", "", false},
		{"no path token", "go.mod dependency metadata references an openmetrics/prometheus client", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, ok := metricsProvenPath(tc.source)
			if ok != tc.wantOK || path != tc.want {
				t.Fatalf("metricsProvenPath(%q) = (%q, %v), want (%q, %v)", tc.source, path, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestDetectedMetricsTargetFailsClosedWithoutProvenPath(t *testing.T) {
	d := appProjectDetection{
		Metrics:       true,
		MetricsSource: "main.go dependency metadata references an openmetrics/prometheus client",
		Ports: []repositoryinspect.PortEvidence{
			{Path: "compose.yaml", Service: "api", Value: "8080:8080"},
		},
	}
	// Dependency-only evidence keeps MetricsPath empty and the conventional
	// default binds.
	source, service, port, path, ok := detectedMetricsTarget(d, []string{"api"})
	if !ok || service != "api" || port != 8080 || path != "/metrics" || source == "" {
		t.Fatalf("conventional default must bind the tuple, got (%q, %q, %d, %q, %v)", source, service, port, path, ok)
	}
	// An arbitrary candidate path must never bind.
	d.MetricsPath = "/health"
	if _, _, _, _, ok := detectedMetricsTarget(d, []string{"api"}); ok {
		t.Fatal("an arbitrary path must never bind")
	}
	d.MetricsPath = "/q/metrics"
	_, service, port, path, ok = detectedMetricsTarget(d, []string{"api"})
	if !ok || service != "api" || port != 8080 || path != "/q/metrics" {
		t.Fatalf("proven path must bind the tuple, got (%q, %d, %q, %v)", service, port, path, ok)
	}
}

// End-to-end: a repo whose only endpoint-shaped paths are health-style routes
// must not have metrics bound in quick mode, and must fail closed if the
// metrics capability was otherwise detected.
func TestQuickInitDoesNotBindHealthPathAsMetrics(t *testing.T) {
	root := t.TempDir()
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), `services:
  api:
    image: example/api
    ports:
      - "8080:8080"
`)
	mustWriteWizardTestFile(t, filepath.Join(root, "main.go"), `package main
func routes() {
	router.GET("/health", healthHandler)
	router.GET("/api/users", usersHandler)
}
`)
	withWizardTestDir(t, root)

	var out bytes.Buffer
	err := appGuidedInitCommand().Run(testOperatorContext(context.Background()), []string{"--quick"}, &out, &bytes.Buffer{})
	if err == nil {
		data, _ := os.ReadFile(filepath.Join(root, application.RepositoryManifestName))
		if strings.Contains(string(data), "metrics:") {
			t.Fatalf("health paths must never bind as metrics endpoints:\n%s", string(data))
		}
		return
	}
	// Failing closed without any unambiguous requirement is the other valid
	// outcome; an ambiguity error that would default to /metrics must not
	// occur for a repo without a proven endpoint.
	if strings.Contains(err.Error(), "metrics endpoint was detected") {
		t.Fatalf("no proven metrics endpoint exists, got: %v", err)
	}
	if !strings.Contains(err.Error(), "no unambiguous application requirements") {
		t.Fatalf("expected fail-closed quick init, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, application.RepositoryManifestName)); !os.IsNotExist(statErr) {
		t.Fatalf("manifest should not be written from unproven paths, stat err=%v", statErr)
	}
}

// The guided prompt path must keep a proven detected path, reject arbitrary
// overrides, and never silently carry an unproven candidate.
func TestPromptMetricsTargetRejectsArbitraryPath(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("api\n8080\n/health\n"))
	var out bytes.Buffer
	_, _, _, err := promptMetricsTarget(reader, &out, []string{"api"}, "/q/metrics")
	if err == nil || !strings.Contains(err.Error(), "not a proven metrics endpoint") {
		t.Fatalf("arbitrary override must be rejected, got %v", err)
	}

	reader = bufio.NewReader(strings.NewReader("api\n8080\n\n"))
	detectedService, detectedPort, path, err := promptMetricsTarget(reader, &out, []string{"api"}, "/q/metrics")
	if err != nil || path != "/q/metrics" || detectedService != "api" || detectedPort != 8080 {
		t.Fatalf("proven detected path must carry through confirmation, got (%q, %q, %d, %v)", detectedService, path, detectedPort, err)
	}

	reader = bufio.NewReader(strings.NewReader("api\n8080\n\n"))
	_, _, path, err = promptMetricsTarget(reader, &out, []string{"api"}, "/health")
	if err != nil || path != "/metrics" {
		t.Fatalf("unproven candidate must never be carried; user must confirm conventional default, got (%q, %v)", path, err)
	}
}
