package repositoryinspect

import (
	"context"
	"strings"
	"testing"
)

// Boundary regression for the metrics endpoint detector: ONLY exact registry
// paths are proven metrics endpoints. Arbitrary candidate paths (health
// checks, API routes) and prefix/substring cases must not produce endpoint
// evidence, so the detection cannot exit open.

func TestMetricsEndpointFromTextAcceptsOnlyRegistryPaths(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		wantOK   bool
		wantPath string
		wantCls  string
	}{
		{"quarkus exact", "quarkus.micrometer.export.prometheus.path=/q/metrics", true, "/q/metrics", "quarkus"},
		{"spring exact", "management.endpoint.prometheus.access=read_only\nmanagement.prometheus.metrics.export.enabled=true\nmanagement.endpoints.web.exposure.include=prometheus", false, "", ""},
		{"spring base path", "management.endpoints.web.base-path=/actuator/prometheus", true, "/actuator/prometheus", "spring"},
		{"conventional exact", `func routes() { router.get("/metrics", metricshandler) }`, true, "/metrics", "conventional"},
		{"health path is not metrics", `func routes() { router.get("/health", healthhandler) }`, false, "", ""},
		{"api users is not metrics", `func routes() { router.get("/api/users", usersHandler) }`, false, "", ""},
		{"prefixed metrics is not conventional", `func routes() { router.get("/metrics-old", h) }`, false, "", ""},
		{"suffixed variant is not registry", `func routes() { router.get("/myorg/metrics", h) }`, false, "", ""},
		{"no path token", "the prometheus client library is exposed here", false, "", ""},
		{"bare word metrics only", "metrics: enabled true", false, "", ""},
		{"path start of string", "/metrics is the endpoint", true, "/metrics", "conventional"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, ok := metricsEndpointFromText(strings.ToLower(tc.text))
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (endpoint %#v)", ok, tc.wantOK, endpoint)
			}
			if !tc.wantOK {
				return
			}
			if endpoint.Path != tc.wantPath || endpoint.Class != tc.wantCls {
				t.Fatalf("endpoint = %#v, want path %q class %q", endpoint, tc.wantPath, tc.wantCls)
			}
		})
	}
}

func TestMetricsPathProvenIsExactMatchOnly(t *testing.T) {
	proven := []string{"/q/metrics", "/actuator/prometheus", "/metrics"}
	for _, path := range proven {
		if !metricsPathProven(path) {
			t.Fatalf("%q must be a proven metrics endpoint", path)
		}
		if metricsPathClass(path) == "other" {
			t.Fatalf("%q must classify against the registry", path)
		}
	}
	notProven := []string{
		"/health", "/healthz", "/api/users", "/metrics-old", "/old-metrics",
		"/myorg/metrics", "/actuator", "/q/metrics-v2", "/METRICS", "metrics", "", "/health/metrics", "/foo/metrics",
	}
	for _, path := range notProven {
		if metricsPathProven(path) {
			t.Fatalf("%q must NOT be a proven metrics endpoint", path)
		}
	}
}

// End-to-end detector regression: a repository whose only endpoint-looking
// paths are health/API routes must not yield a detected metrics endpoint.
func TestOpenMetricsDetectorFailsClosedOnArbitraryPaths(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "compose.yaml", "services:\n  api:\n    image: example/api\n    ports: [\"8080:8080\"]\n")
	writeTestFile(t, root, "api.go", `package api

func routes() {
	router.GET("/health", healthHandler)
	router.GET("/api/users", usersHandler)
	router.GET("/metrics-old", legacyHandler)
}
`)
	result, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range result.Findings {
		if finding.Capability != "metrics" {
			continue
		}
		for _, evidence := range finding.Evidence {
			if evidence.Kind == EvidenceEndpoint {
				t.Fatalf("arbitrary paths produced endpoint evidence: %#v", finding.Evidence)
			}
		}
		if finding.Confidence == ConfidenceDetected {
			t.Fatalf("metrics must not be detected from arbitrary paths: %#v", finding)
		}
	}
}

// A proven registry path still yields exact endpoint evidence.
func TestOpenMetricsDetectorStillCarriesProvenRegistryPaths(t *testing.T) {
	cases := []struct {
		file   string
		source string
		want   string
	}{
		{"api.go", "package api\nfunc routes() { router.GET(\"/metrics\", metricsHandler) }\n", "/metrics"},
		{"application.properties", "quarkus.micrometer.export.prometheus.path=/q/metrics\n", "/q/metrics"},
		{"application.properties", "management.endpoints.web.base-path=/actuator/prometheus\n", "/actuator/prometheus"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			root := t.TempDir()
			writeTestFile(t, root, "compose.yaml", "services:\n  api:\n    image: example/api\n    ports: [\"8080:8080\"]\n")
			writeTestFile(t, root, tc.file, tc.source)
			result, err := Inspect(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}
			var finding *Finding
			for i := range result.Findings {
				if result.Findings[i].Capability == "metrics" {
					finding = &result.Findings[i]
				}
			}
			if finding == nil || finding.Confidence != ConfidenceDetected {
				t.Fatalf("metrics must be detected for %s, got %#v", tc.want, finding)
			}
			var endpoint string
			for _, evidence := range finding.Evidence {
				if evidence.Kind == EvidenceEndpoint && strings.Contains(evidence.Detail, tc.want) {
					endpoint = tc.want
				}
			}
			if endpoint == "" {
				t.Fatalf("endpoint evidence must carry exact path %s, got %#v", tc.want, finding.Evidence)
			}
		})
	}
}
