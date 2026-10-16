package main

import (
	"strings"
	"os/exec"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestAppChangePreviewConsequences(t *testing.T) {
	location := application.ResourceLocation{Owner: "app", Target: "dev", Kind: "volume", Reference: "data-volume"}
	plan := application.ProviderImpactPlan{Target: "dev", Complete: false, Destructive: true, ConfirmationsRequired: 2, Impacts: []application.ProviderImpact{
		{InstanceID: "sql", Operation: "stop", Scenario: "provider-still-used", Before: application.ProviderRealization{State: "running", Locations: []application.ResourceLocation{location}},
			Functions: []application.FunctionConsequence{{Consumer: "api", Function: "database.query", Outcome: "unavailable", Required: true}},
			Materials: []application.MaterialConsequence{{Before: application.ProviderMaterial{ID: "data", Kind: "volume", Location: &location}, After: application.MaterialDisposition{MaterialID: "data", Disposition: "retain"}}},
			Prerequisites: []application.ProviderPrerequisite{{Kind: "consumer-rebind", Subject: "api", Supported: false, Verified: false}},
			Effects: []application.OperationalConsequence{{Aspect: "availability", Before: "online", After: "offline"}}},
		{InstanceID: "identity", Operation: "replace", Scenario: "provider-replaced", Functions: []application.FunctionConsequence{{Consumer: "web", Function: "identity.refresh", Outcome: "unknown", ValidUntil: "2026-10-17T12:00:00Z"}}},
	}}
	text := appChangePreview(plan)
	for _, part := range []string{"stop", "replace", "provider-still-used", "api", "database.query", "data-volume", "retain", "consumer-rebind", "identity.refresh", "2026-10-17", "2 confirmation", "incomplete", "availability"} {
		if !strings.Contains(text, part) { t.Errorf("missing %q in %s", part, text) }
	}
}

func TestAppChangePreviewAddRemoveDeselection(t *testing.T) {
	for _, row := range []struct{ operation, scenario string }{
		{"add", "provider-changed"}, {"destroy", "capability-no-longer-requested"}, {"remove-runtime", "unused-provider"}, {"replace", "provider-replaced"},
	} {
		p := application.ProviderImpactPlan{Target: "dev", ConfirmationsRequired: 2, Impacts: []application.ProviderImpact{{InstanceID: "p", Operation: row.operation, Scenario: row.scenario}}}
		text := appChangePreview(p)
		if !strings.Contains(text, row.operation) || !strings.Contains(text, row.scenario) { t.Fatal(text) }
	}
}

func TestAppChangePreviewSecretProtectionAndNoInventedOutcome(t *testing.T) {
	p := application.ProviderImpactPlan{Target: "dev", ConfirmationsRequired: 2, Complete: false, Impacts: []application.ProviderImpact{{InstanceID: "secrets", Operation: "stop", Before: application.ProviderRealization{Evidence: "TOP-SECRET", Access: &application.ProviderAccess{Endpoint: "secret-token-endpoint"}}, Materials: []application.MaterialConsequence{{Before: application.ProviderMaterial{ID: "lease", Evidence: "PRIVATE-CREDENTIAL"}}}}}}
	got := appChangePreview(p)
	for _, secret := range []string{"TOP-SECRET", "secret-token-endpoint", "PRIVATE-CREDENTIAL"} {
		if strings.Contains(got, secret) { t.Fatalf("secret exposed: %s", secret) }
	}
	if strings.Contains(got, "backup available") || strings.Contains(got, "approved") { t.Fatal("invented consequence") }
}

func TestAppChangePreviewFormatting(t *testing.T) {
	for _, name := range []string{"app_change_preview.go", "app_change_preview_test.go", "app_change_result_presenter.go", "app_change_result_presenter_test.go"} {
		diff, err := exec.Command("gofmt", "-d", name).CombinedOutput()
		if err != nil || len(diff) != 0 {
			t.Fatalf("gofmt mismatch in %s: %s (%v)", name, diff, err)
		}
	}
}
