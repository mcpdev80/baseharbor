package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func persesAlertEvaluatorManifest() application.Manifest {
	m := application.New("perses-alerts-demo", "dev", false, false, false)
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true
	return application.WithMetricsSource(m, "default", "api", 8080, "/metrics")
}

func TestBuildPersesAlertEvaluatorRequiresSelectionAndMetricInput(t *testing.T) {
	m := persesAlertEvaluatorManifest()
	result, err := BuildPersesAlertEvaluator(m)
	if err != nil {
		t.Fatal(err)
	}
	if result.Reachability != PersesAlertReachabilityConfiguredUnverified || result.Evaluator.Owner != string(capability.ProviderPrometheus) || result.Evaluator.Evaluator != PersesAlertEvaluatorKind {
		t.Fatalf("unexpected Prometheus evaluator contract: %+v", result)
	}

	m.Metrics.Sources = nil
	if _, err := BuildPersesAlertEvaluator(m); err == nil {
		t.Fatal("no configured metrics input must not advertise alert evaluation")
	}
	m = persesAlertEvaluatorManifest()
	m.Services.Perses = false
	if _, err := BuildPersesAlertEvaluator(m); err == nil {
		t.Fatal("unselected Perses must fail closed")
	}
}

func TestVerifyPrometheusAlertRuleAPIAcceptsRecentHealthyEvaluation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rules" || r.URL.Query().Get("type") != "alert" {
			t.Errorf("request = %s, want alert rules API", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"success","data":{"groups":[{"name":"baseharbor-target-health","rules":[{"name":"BaseHarborTargetUnavailable","query":"up{job=~\"baseharbor-applications|baseharbor-providers\"} == 0","health":"ok","lastError":"","lastEvaluation":%q}]}]}}`, time.Now().UTC().Format(time.RFC3339Nano))
	}))
	defer server.Close()
	if err := verifyPrometheusAlertRuleAPI(context.Background(), server.Client(), server.URL); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyPrometheusAlertRuleAPIFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "rule absent", body: `{"status":"success","data":{"groups":[]}}`},
		{name: "unhealthy evaluator", body: `{"status":"success","data":{"groups":[{"name":"baseharbor-target-health","rules":[{"name":"BaseHarborTargetUnavailable","health":"err","lastError":"parse error","lastEvaluation":"2026-11-21T11:00:00Z"}]}]}}`},
		{name: "stale evaluation", body: `{"status":"success","data":{"groups":[{"name":"baseharbor-target-health","rules":[{"name":"BaseHarborTargetUnavailable","health":"ok","lastEvaluation":"2020-01-01T00:00:00Z"}]}]}}`},
		{name: "failed API", body: `{"status":"error","data":{"groups":[]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			if err := verifyPrometheusAlertRuleAPI(context.Background(), server.Client(), server.URL); err == nil {
				t.Fatal("unverified rule evaluation was accepted")
			}
		})
	}
}

func TestPersesAlertEvaluatorJSONContainsNoNotificationCredentials(t *testing.T) {
	data, err := PersesAlertEvaluatorJSON(PersesAlertEvaluator{
		Kind: PersesAlertEvaluatorKind, Name: "baseharbor-observability-alerts",
		Evaluator: PersesAlertEvaluatorKind, Owner: string(capability.ProviderPrometheus),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) || strings.Contains(string(data), "recipient") || strings.Contains(string(data), "password") {
		t.Fatalf("evaluator contract includes unrelated notification data: %s", data)
	}
}
