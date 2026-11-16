package main

import (
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	"github.com/mcpdev80/baseharbor/internal/traces"
)

func TestRetainedTempoVersionChangeRequiresConcreteCurrentConsent(t *testing.T) {
	r := retainedNativeBackendFixture(t)
	r.Manifest = application.WithOTLPTelemetry(r.Manifest, "traces")
	if err := application.ReconcileReferenceProviderRegistryAt(r.TargetStateRoot, r.Manifest, capability.Resource{Application: r.Manifest.Name, Kind: capability.Traces, Name: "default", Provider: capability.ProviderTempo}); err != nil {
		t.Fatal(err)
	}
	files, _, err := traces.EnsureProviderFilesAt(t.Context(), serviceissuer.New(t), r.TargetStateRoot, r.Target.Name, r.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	previous := strings.ReplaceAll(string(canonical), "image: "+traces.ProviderImage+"\n", "image: docker.io/grafana/tempo:3.0.2\n")
	if err := os.WriteFile(files.Compose, []byte(previous), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := standaloneProviderVersionChangeRequests(t.Context(), r, nil)
	if err != nil || len(requests) != 1 || requests[0].Operation != "update" {
		t.Fatalf("retained version silently changed without current request: %+v %v", requests, err)
	}
	plan, err := buildResolvedApplicationPlan(t.Context(), r)
	if err != nil || plan.ProviderPlan.Impact == nil || plan.ProviderPlan.Impact.ConfirmationsRequired != 2 {
		t.Fatalf("unknown version consequences lost both acknowledgements: %v", err)
	}
	encoded := false
	for _, impact := range plan.ProviderPlan.Impact.Impacts {
		for _, effect := range impact.Effects {
			if effect.Aspect == "provider-version" && strings.Contains(effect.Before, "3.0.2") && strings.Contains(effect.After, traces.ProviderImage) && strings.Contains(effect.After, "vParquet4") {
				encoded = true
			}
		}
	}
	if !encoded {
		t.Fatal("concrete old/new version and retained write format absent")
	}
	if err := application.ValidateProviderChangeConsent(*plan.ProviderPlan.Impact, application.ProviderChangeConsent{PlanID: plan.ProviderPlan.Impact.PlanID, First: true}); err == nil {
		t.Fatal("ordinary/single approval granted the version change")
	}
	after, _ := os.ReadFile(files.Env)
	actual, _ := os.ReadFile(files.Compose)
	if string(before) != string(after) || string(actual) != previous {
		t.Fatal("version planning changed retained definition or credentials")
	}
	duplicate, err := standaloneProviderVersionChangeRequests(t.Context(), r, requests)
	if err != nil || len(duplicate) != 1 {
		t.Fatal("combined version request duplicated same provider", err)
	}
	if err := os.WriteFile(files.Compose, []byte(previous+"# modified owner\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := standaloneProviderVersionChangeRequests(t.Context(), r, nil); err == nil {
		t.Fatal("modified provider owner granted version update authority")
	}
}
