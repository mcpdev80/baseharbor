package model

import (
	"reflect"
	"strings"
	"testing"
)

func TestWorkloadPlanStructurallyExcludesSourceAndBuildSemantics(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(WorkloadPlan{}),
		reflect.TypeOf(Service{}),
	} {
		for i := 0; i < typ.NumField(); i++ {
			name := strings.ToLower(typ.Field(i).Name)
			for _, forbidden := range []string{"build", "dockerfile", "context", "source", "compose", "namespace"} {
				if strings.Contains(name, forbidden) {
					t.Fatalf("%s exposes forbidden runtime-boundary field %q", typ.Name(), typ.Field(i).Name)
				}
			}
		}
	}
}

func TestWorkloadPlanRequiresResolvedImageAndOpaqueScope(t *testing.T) {
	plan := WorkloadPlan{
		Application: "demo",
		Environment: "dev",
		Target:      Target{Scope: "opaque-target"},
		Services: []Service{{
			Name:  "web",
			Image: "registry.example/demo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}},
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("valid runtime plan: %v", err)
	}

	plan.Services[0].Image = ""
	if err := plan.Validate(); err == nil {
		t.Fatal("expected unresolved image to be rejected")
	}
}
