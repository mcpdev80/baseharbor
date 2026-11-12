package main

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestRemoteDestroyPlanRequiresSeparateDataDeletionApproval(t *testing.T) {
	ctx, target, _ := remoteJourneyFixture(t)
	plan := application.ProviderImpactPlan{
		Version: application.ProviderImpactVersion, PlanID: "destroy-plan", Target: "local",
		ConfirmationsRequired: 2, Complete: true, Destructive: true,
		Impacts: []application.ProviderImpact{{InstanceID: "demo-sql", Operation: "remove-runtime"}},
	}
	cause := &machine.Error{Code: machine.ErrorApprovalRequired, CauseCode: "impact-confirmation-required", Details: plan}
	var output bytes.Buffer
	next, retry, err := acknowledgeRemoteDestroyChallenge(ctx, target, cause, bufio.NewReader(strings.NewReader("y\ny\n")), &output, map[string]bool{}, true)
	if err != nil || !retry || !strings.Contains(output.String(), "Remove this application") || !strings.Contains(output.String(), "Permanently delete") {
		t.Fatalf("destroy confirmation failed: retry=%v err=%v output=%s", retry, err, output.String())
	}
	consent, err := invocationProviderChangeConsent(next)
	if err != nil || consent == nil || consent.PlanID != plan.PlanID || !consent.First || !consent.Second || !consent.DeleteData {
		t.Fatalf("wrong destroy consent: %+v %v", consent, err)
	}
}

func TestRemoteDestroyAcknowledgesApplicationScopePlanID(t *testing.T) {
	ctx, target, _ := remoteJourneyFixture(t)
	impact := application.ProviderImpactPlan{
		Version: application.ProviderImpactVersion, PlanID: "leaf-plan", Target: "local",
		ConfirmationsRequired: 1, Complete: true,
		Impacts: []application.ProviderImpact{{InstanceID: "demo-sql", Operation: "stop"}},
	}
	scope, err := application.ComposeProviderImpactScope(application.ProviderScopeOwner{
		Owner: "application-deployment", Reference: "/protected/deployment.json", Subject: "deployment-id", Revision: "revision", Destructive: true,
	}, []application.ProviderImpactPlan{impact}, true)
	if err != nil {
		t.Fatal(err)
	}
	plan := applicationDestructionPlan{
		Scope:     scope,
		Providers: application.ProviderPlan{Impact: &impact},
		Effects:   []application.OperationalConsequence{{Aspect: "application-destruction", After: "Delete app-owned state."}},
	}
	cause := &machine.Error{Code: machine.ErrorApprovalRequired, CauseCode: "impact-confirmation-required", Details: plan}
	var output bytes.Buffer
	next, retry, err := acknowledgeRemoteDestroyChallenge(ctx, target, cause, bufio.NewReader(strings.NewReader("y\ny\n")), &output, map[string]bool{}, true)
	if err != nil || !retry {
		t.Fatalf("application-scope destroy plan was not acknowledged: retry=%v err=%v output=%s", retry, err, output.String())
	}
	consent, err := invocationProviderChangeConsent(next)
	if err != nil || consent == nil || consent.PlanID != scope.PlanID || consent.PlanID == impact.PlanID || !consent.First || !consent.Second || !consent.DeleteData {
		t.Fatalf("destroy consent must bind to the outer destructive scope: consent=%+v scope=%q leaf=%q err=%v", consent, scope.PlanID, impact.PlanID, err)
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
	_, retry, err := acknowledgeRemoteDestroyChallenge(ctx, target, cause, bufio.NewReader(strings.NewReader("y\nn\n")), &output, map[string]bool{}, true)
	if err == nil || retry || !strings.Contains(output.String(), "Removal cancelled") || !strings.Contains(output.String(), "Permanently delete") {
		t.Fatalf("data deletion was not separately declined: retry=%v err=%v output=%s", retry, err, output.String())
	}
}

func TestRemoteStopReviewsProviderChangesButRetainsData(t *testing.T) {
	ctx, target, _ := remoteJourneyFixture(t)
	plan := application.ProviderImpactPlan{
		Version: application.ProviderImpactVersion, PlanID: "stop-plan", Target: "local",
		ConfirmationsRequired: 2, Complete: true,
		Impacts: []application.ProviderImpact{{InstanceID: "demo-sql", Operation: "stop"}},
	}
	cause := &machine.Error{Code: machine.ErrorApprovalRequired, CauseCode: "impact-confirmation-required", Details: plan}
	var output bytes.Buffer
	next, retry, err := acknowledgeRemoteDestroyChallenge(ctx, target, cause, bufio.NewReader(strings.NewReader("y\n")), &output, map[string]bool{}, false)
	if err != nil || !retry || !strings.Contains(output.String(), "retaining its data") || strings.Contains(output.String(), "Permanently delete") {
		t.Fatalf("stop consent did not preserve data: retry=%v err=%v output=%s", retry, err, output.String())
	}
	consent, err := invocationProviderChangeConsent(next)
	if err != nil || consent == nil || !consent.First || !consent.Second || consent.DeleteData {
		t.Fatalf("stop consent authorized data deletion: %+v %v", consent, err)
	}
	var result bytes.Buffer
	receipt := []byte(`{"application":"demo","environment":"dev","target":"dev-docker-1","verified":true,"state":"runtime-retired","data_deleted":false}`)
	if err := renderRemoteDestroyCompletion(&result, receipt, true); err != nil || !strings.Contains(result.String(), "persistent data and recovery owner retained") {
		t.Fatalf("stop receipt did not report retained data: %v %q", err, result.String())
	}
}

func TestRemoteDestroyCompletionReportsDataDisposition(t *testing.T) {
	for _, tc := range []struct {
		name, receipt, expected string
	}{
		{"deleted", "{\"application\":\"demo\",\"environment\":\"dev\",\"target\":\"dev-docker-1\",\"verified\":true,\"data_deleted\":true}", "Application-owned data was permanently deleted."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := renderRemoteDestroyCompletion(&out, []byte(tc.receipt), false); err != nil || !strings.Contains(out.String(), tc.expected) {
				t.Fatalf("wrong completion output: %v %q", err, out.String())
			}
		})
	}
}
