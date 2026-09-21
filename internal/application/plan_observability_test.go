package application

import (
	"strings"
	"testing"
)

func TestBuildPlanSupportsCurrentObservabilityCapabilities(t *testing.T) {
	m := Manifest{
		Version:     CurrentVersion,
		Name:        "demo",
		Environment: "dev",
		Workload: WorkloadConfig{
			Compose:  "compose.yaml",
			Services: []string{"api"},
		},
		Metrics: MetricsRequirements{Sources: []MetricsSourceRequirement{
			{Name: "application", Service: "api", Port: 8080, Path: "/metrics"},
		}},
		Logs: LogsRequirements{Collect: []string{"api"}},
	}
	m = WithOTLPTelemetry(m, "traces")

	plan, err := BuildPlan(m)
	if err != nil {
		t.Fatal(err)
	}

	joined := make([]string, 0, len(plan.Actions))
	for _, action := range plan.Actions {
		joined = append(joined, action.Resource)
	}
	got := strings.Join(joined, ",")
	for _, expected := range []string{
		"telemetry.otlp:default",
		"metrics:application",
		"logs:api",
		"workload",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("plan resources %q do not contain %q", got, expected)
		}
	}
}
