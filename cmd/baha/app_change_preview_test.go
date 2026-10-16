package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestAppChangePreviewConsequences(t *testing.T) {
	location := application.ResourceLocation{Owner: "app", Target: "dev", Kind: "volume", Reference: "data-volume"}
	plan := application.ProviderImpactPlan{Target: "dev", Complete: false, Destructive: true, ConfirmationsRequired: 2, Impacts: []application.ProviderImpact{
		{InstanceID: "sql", Operation: "stop", Scenario: "provider-still-used", Before: application.ProviderRealization{State: "running", Locations: []application.ResourceLocation{location}},
			Functions:     []application.FunctionConsequence{{Consumer: "api", Function: "database.query", Outcome: "unavailable", Required: true}},
			Materials:     []application.MaterialConsequence{{Before: application.ProviderMaterial{ID: "data", Kind: "volume", Location: &location}, After: application.MaterialDisposition{MaterialID: "data", Disposition: "retain"}}},
			Prerequisites: []application.ProviderPrerequisite{{Kind: "consumer-rebind", Subject: "api", Supported: false, Verified: false}},
			Effects:       []application.OperationalConsequence{{Aspect: "availability", Before: "online", After: "offline"}}},
		{InstanceID: "identity", Operation: "replace", Scenario: "provider-replaced", Functions: []application.FunctionConsequence{{Consumer: "web", Function: "identity.refresh", Outcome: "unknown", ValidUntil: "2026-10-17T12:00:00Z"}}},
	}}
	text := appChangePreview(plan)
	for _, part := range []string{"stop", "replace", "provider-still-used", "api", "database.query", "data-volume", "retain", "consumer-rebind", "identity.refresh", "2026-10-17", "2 confirmation", "incomplete", "availability"} {
		if !strings.Contains(text, part) {
			t.Errorf("missing %q in %s", part, text)
		}
	}
}

func TestAppChangePreviewAddRemoveDeselection(t *testing.T) {
	for _, row := range []struct{ operation, scenario string }{
		{"add", "provider-changed"}, {"destroy", "capability-no-longer-requested"}, {"remove-runtime", "unused-provider"}, {"replace", "provider-replaced"},
	} {
		p := application.ProviderImpactPlan{Target: "dev", ConfirmationsRequired: 2, Impacts: []application.ProviderImpact{{InstanceID: "p", Operation: row.operation, Scenario: row.scenario}}}
		text := appChangePreview(p)
		if !strings.Contains(text, row.operation) || !strings.Contains(text, row.scenario) {
			t.Fatal(text)
		}
	}
}

func TestAppChangePreviewSecretProtectionAndNoInventedOutcome(t *testing.T) {
	p := application.ProviderImpactPlan{Target: "dev", ConfirmationsRequired: 2, Complete: false, Impacts: []application.ProviderImpact{{InstanceID: "secrets", Operation: "stop", Before: application.ProviderRealization{Evidence: "TOP-SECRET", Access: &application.ProviderAccess{Endpoint: "secret-token-endpoint"}}, Materials: []application.MaterialConsequence{{Before: application.ProviderMaterial{ID: "lease", Evidence: "PRIVATE-CREDENTIAL"}}}}}}
	got := appChangePreview(p)
	for _, secret := range []string{"TOP-SECRET", "secret-token-endpoint", "PRIVATE-CREDENTIAL"} {
		if strings.Contains(got, secret) {
			t.Fatalf("secret exposed: %s", secret)
		}
	}
	if strings.Contains(got, "backup available") || strings.Contains(got, "approved") {
		t.Fatal("invented consequence")
	}
}

func TestAppChangePreviewFormatting(t *testing.T) {
	for _, name := range []string{"app_change_preview.go", "app_change_preview_test.go", "app_change_result_presenter.go", "app_change_result_presenter_test.go"} {
		diff, err := exec.Command("gofmt", "-d", name).CombinedOutput()
		if err != nil || len(diff) != 0 {
			t.Errorf("gofmt mismatch in %s: %s (%v)", name, diff, err)
		}
	}
}

func TestAppChangePreviewAuthoritativeRenewalAndSecurityConsequences(t *testing.T) {
	remaining := int64(7200)
	secure := &application.ProviderAccess{
		Method: "sensitive-method-do-not-print", Endpoint: "https://token-secret@example.invalid",
		Security: capability.SecureBinding{
			Credentials: []capability.CredentialReference{{Name: "hidden-credential", Reference: "baseharbor://secrets/hidden"}},
			Trust: []capability.TrustMaterialReference{{Name: "hidden-ca", Reference: "baseharbor://trust/hidden"}},
			Lifecycle: capability.SecurityLifecycleSupport{Renewable: true, Rotatable: true},
		},
	}
	changed := &application.ProviderAccess{Security: capability.SecureBinding{Lifecycle: capability.SecurityLifecycleSupport{Renewable: false, Revocable: true}}}
	p := application.ProviderImpactPlan{Target: "dev", Destructive: true, ConfirmationsRequired: 2, Impacts: []application.ProviderImpact{{
		InstanceID: "pki", Operation: "stop", Before: application.ProviderRealization{State: "running", Access: secure},
		After: &application.ProviderRealization{State: "stopped", Access: changed},
		Functions: []application.FunctionConsequence{{Consumer: "web", Function: "pki.validate", Outcome: "changed", Behavior: "offline-until-expiry", ValidUntil: "2026-11-01T00:00:00Z", RemainingValiditySeconds: &remaining}},
		Materials: []application.MaterialConsequence{{
			Before: application.ProviderMaterial{ID: "cert", Kind: "certificate", ValidFrom: "2026-10-01T00:00:00Z", ExpiresAt: "2026-11-01T00:00:00Z", Access: secure},
			After: application.MaterialDisposition{MaterialID: "cert", Disposition: "retain", AccessAfter: changed},
			Meaning: "future renewal is not implied by retention", RemainingValiditySeconds: &remaining,
		}},
		Prerequisites: []application.ProviderPrerequisite{{Kind: "consumer-rebind", Subject: "web", Consequence: "without verified rebind, consumer continuity is not claimed", Supported: false}},
	}}}
	got := appChangePreview(p)
	for _, want := range []string{
		"future renewal is not implied by retention", "offline-until-expiry", "Remaining function validity (seconds): 7200",
		"Remaining material validity (seconds): 7200", "Material valid from: 2026-10-01", "Material expires at: 2026-11-01",
		"without verified rebind, consumer continuity is not claimed", "Before access: reported",
		"credential references: 1", "trust references: 1", "renewable: true",
		"Proposed access (unverified): reported", "renewable: false", "revocable: true", "2 confirmation",
	} {
		if !strings.Contains(got, want) { t.Errorf("missing authoritative explanation %q: %s", want, got) }
	}
	for _, secret := range []string{"sensitive-method-do-not-print", "token-secret", "hidden-credential", "hidden-ca", "baseharbor://"} {
		if strings.Contains(got, secret) { t.Fatalf("sensitive access leaked %q", secret) }
	}
}

func TestAppChangePreviewContinuationAndDestructiveAlternative(t *testing.T) {
	plan := application.ProviderImpactPlan{Target: "prod", Destructive: true, ConfirmationsRequired: 2, Impacts: []application.ProviderImpact{
		{InstanceID: "old", Operation: "replace", Scenario: "provider-replaced",
			Functions: []application.FunctionConsequence{{Consumer: "api", Function: "database.query", Outcome: "changed", Behavior: "reconnect-verified"}},
			Prerequisites: []application.ProviderPrerequisite{{Kind: "consumer-rebind", Consequence: "rebind must be verified", Supported: true, Verified: true}}},
		{InstanceID: "unused", Operation: "destroy", Scenario: "capability-no-longer-requested",
			Materials: []application.MaterialConsequence{{Before: application.ProviderMaterial{ID: "volume"}, After: application.MaterialDisposition{Disposition: "delete"}, Meaning: "deletion is irreversible"}}},
	}}
	got := appChangePreview(plan)
	for _, want := range []string{"reconnect-verified", "rebind must be verified", "deletion is irreversible", "capability-no-longer-requested"} {
		if !strings.Contains(got, want) { t.Errorf("missing %q", want) }
	}
}
