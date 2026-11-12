package main

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestRemoteDestroyPlanUsesOneExplicitReviewAndRetainsData(t *testing.T) {
	ctx, target, _ := remoteJourneyFixture(t)
	plan := application.ProviderImpactPlan{
		Version: application.ProviderImpactVersion, PlanID: "destroy-plan", Target: "local",
		ConfirmationsRequired: 2, Complete: true,
		Impacts: []application.ProviderImpact{{InstanceID: "demo-sql", Operation: "remove-runtime"}},
	}
	cause := &machine.Error{Code: machine.ErrorApprovalRequired, CauseCode: "impact-confirmation-required", Details: plan}
	var output bytes.Buffer
	next, retry, err := acknowledgeRemoteDestroyChallenge(ctx, target, cause, bufio.NewReader(strings.NewReader("y\n")), &output, map[string]bool{})
	if err != nil || !retry || !strings.Contains(output.String(), "Remove this application") {
		t.Fatalf("destroy confirmation failed: retry=%v err=%v output=%s", retry, err, output.String())
	}
	consent, err := invocationProviderChangeConsent(next)
	if err != nil || consent == nil || consent.PlanID != plan.PlanID || !consent.First || !consent.Second || consent.DeleteData {
		t.Fatalf("wrong retained-data consent: %+v %v", consent, err)
	}
}

func TestRemoteFullResetRequiresSeparateDataConsent(t *testing.T) {
	ctx, target, _ := remoteJourneyFixture(t)
	plan := application.ProviderImpactPlan{
		Version: application.ProviderImpactVersion, PlanID: "reset-plan", Target: "local",
		ConfirmationsRequired: 2, Complete: true, Destructive: true,
		Impacts: []application.ProviderImpact{{InstanceID: "demo-sql", Operation: "destroy"}},
	}
	cause := &machine.Error{Code: machine.ErrorApprovalRequired, CauseCode: "data-deletion-consent-required", Details: plan}
	var output bytes.Buffer
	_, retry, err := acknowledgeRemoteDestroyChallenge(ctx, target, cause, bufio.NewReader(strings.NewReader("y\nn\n")), &output, map[string]bool{})
	if err == nil || retry || !strings.Contains(output.String(), "Removal cancelled") || !strings.Contains(output.String(), "Permanently delete") {
		t.Fatalf("data deletion was not separately declined: retry=%v err=%v output=%s", retry, err, output.String())
	}
}

func TestRemoteDestroyCompletionReportsDataDisposition(t *testing.T) {
	for _, tc := range []struct {
		name, receipt, expected string
	}{
		{"retained", "{\"application\":\"demo\",\"environment\":\"dev\",\"target\":\"dev-docker-1\",\"verified\":true}", "Persistent data and recovery material were retained."},
		{"deleted", "{\"application\":\"demo\",\"environment\":\"dev\",\"target\":\"dev-docker-1\",\"verified\":true,\"data_deleted\":true}", "Application data was permanently deleted as requested."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := renderRemoteDestroyCompletion(&out, []byte(tc.receipt)); err != nil || !strings.Contains(out.String(), tc.expected) {
				t.Fatalf("wrong completion output: %v %q", err, out.String())
			}
		})
	}
}
