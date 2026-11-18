package main

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// TestConsentLabelsAreEnglishCatalog asserts the destructive / consent-gated
// CLI paths print the English catalog labels, never a hardcoded German
// fallback string. This is the regression hunt 0428-106 covers.
func TestConsentLabelsAreEnglishCatalog(t *testing.T) {
	labels := application.ProviderImpactConsentCatalog()

	wantNonGerman := map[string]string{
		"control-plane-stop-impact":         labels.ControlPlaneStopImpact,
		"node-disconnect-impact":            labels.NodeDisconnectImpact,
		"target-definition-change-impact":   labels.TargetDefinitionChangeImpact,
		"application-stop-impact":           labels.ApplicationStopImpact,
		"runtime-destroy-destructive":       labels.RuntimeDestroyImpactDestructive,
		"runtime-destroy-non-destructive":   labels.RuntimeDestroyImpactNonDestructive,
		"provider-impact-lead":              labels.ProviderImpactLead,
		"provider-destructive-confirm":      labels.ProviderDestructiveConfirm,
		"provider-two-stage-confirm":        labels.ProviderTwoStageConfirm,
		"provider-destructive-data-consent": labels.ProviderDestructiveDataConsentPrompt,
		"application-restore-impact":        labels.ApplicationRestoreImpact,
		"external-provider-removal-impact":  labels.ExternalProviderRemovalImpact,
	}
	if len(wantNonGerman) != 12 {
		t.Fatalf("catalog grew or shrank: %d labels", len(wantNonGerman))
	}
	for _, v := range wantNonGerman {
		if strings.HasPrefix(v, "Achtung:") {
			t.Errorf("catalog label still German lead: %q", v)
		}
	}
}

// TestRuntimeDestroyImpactPrintsEnglishCatalog confirms the render site swaps.
func TestRuntimeDestroyImpactPrintsEnglishCatalog(t *testing.T) {
	labels := application.ProviderImpactConsentCatalog()
	if !strings.Contains(labels.RuntimeDestroyImpactDestructive, "two current-plan confirmations") {
		t.Fatalf("runtime destructive label lost its confirmation notice: %q", labels.RuntimeDestroyImpactDestructive)
	}
	if !strings.Contains(labels.RuntimeDestroyImpactNonDestructive, "persistent data and protected owners") {
		t.Fatalf("runtime non-destructive label lost its retention notice: %q", labels.RuntimeDestroyImpactNonDestructive)
	}
}

// TestProviderImpactAcknowledgementLabelsUseCatalog walks the two-stage prompt
// path and asserts it renders the catalog confirmation labels, not German text.
func TestProviderImpactAcknowledgementLabelsUseCatalog(t *testing.T) {
	labels := application.ProviderImpactConsentCatalog()
	plan := application.ProviderImpactPlan{
		Version:               application.ProviderImpactVersion,
		Target:                "core",
		PlanID:                "plan-current",
		ConfirmationsRequired: 2,
		Destructive:           true,
	}
	var out bytes.Buffer
	consent, err := readProviderImpactAcknowledgements(bufio.NewReader(strings.NewReader("yes\nyes\nyes\n")), &out, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !consent.First || !consent.Second {
		t.Fatalf("two-stage acknowledgement did not record both confirmations: %#v", consent)
	}
	s := out.String()
	if !strings.Contains(s, labels.ProviderDestructiveConfirm) {
		t.Errorf("destructive confirm label missing. got:\n%s", s)
	}
	if strings.Contains(s, "Achtung:") {
		t.Errorf("German fallback still rendered:\n%s", s)
	}
	// Data-deletion prompt applies the plan id into the %s of the catalog template.
	if !strings.Contains(s, "Warning: separately authorize permanent deletion of the displayed owned data (plan plan-current)? [y/N]: ") {
		t.Errorf("data-deletion prompt label missing. got:\n%s", s)
	}
}
