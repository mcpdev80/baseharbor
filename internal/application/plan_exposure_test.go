package application

import "testing"

func TestBuildPlanIncludesManagedHTTPExposure(t *testing.T) {
	m := Manifest{
		Version:     CurrentVersion,
		Name:        "frontend",
		Environment: "dev",
		Workload:    WorkloadConfig{Compose: "compose.yaml", Services: []string{"web"}},
		Exposures:   []HTTPExposureRequirement{{Name: "public", Service: "web", Port: 8080, Protocol: "http", Visibility: "public"}},
	}
	plan, err := BuildPlan(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range plan.Actions {
		if action.Kind == "ensure" && action.Resource == "exposure:public" {
			return
		}
	}
	t.Fatalf("managed exposure action missing from plan: %#v", plan.Actions)
}
