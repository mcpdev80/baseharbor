package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestProviderImpactHumanOutputRetainsConcreteConsequences(t *testing.T) {
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "core", PlanID: "plan-current", ConfirmationsRequired: 2, Destructive: true,
		Impacts: []application.ProviderImpact{{InstanceID: "postgres-owned", Operation: "remove-runtime",
			Functions: []application.FunctionConsequence{{Consumer: "demo/dev", Function: "database.sql", Required: true, Outcome: "unavailable"}},
			Materials: []application.MaterialConsequence{{Before: application.ProviderMaterial{ID: "database-volume", Location: &application.ResourceLocation{Reference: "volume-owned", Target: "core"}}, After: application.MaterialDisposition{Disposition: "delete"}, Meaning: "Application records will be permanently removed."}},
			Effects:   []application.OperationalConsequence{{Aspect: "sessions", Before: "active", After: "invalidated"}},
		}}}
	var out bytes.Buffer
	if err := printProviderImpactForCLI(context.Background(), &out, plan); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Installation on core", "permanently deleted", "could not be fully checked", "Remove SQL", "SQL will be unavailable for demo/dev", "database-volume -> delete", "permanently removed", "sessions: active -> invalidated"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing consequence %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), `"prerequisites":`) {
		t.Fatal("normal CLI printed raw plan JSON")
	}
	out.Reset()
	ctx := cli.WithOutputOptions(context.Background(), cli.OutputOptions{Verbose: true})
	if err := printProviderImpactForCLI(ctx, &out, plan); err != nil || !strings.Contains(out.String(), `"plan_id": "plan-current"`) {
		t.Fatal("verbose output lost the structured plan", err)
	}
}

func TestProviderImpactAllowsExplicitAlternativeLessDeletion(t *testing.T) {
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, PlanID: "concrete-effects", Complete: false, Attention: "no destination; deletion", ConfirmationsRequired: 2, Destructive: true}
	for _, answers := range []string{"no\n", "\n", "", "maybe\n"} {
		consent, _ := readProviderApplyAcknowledgement(bufio.NewReader(strings.NewReader(answers)), new(bytes.Buffer), plan)
		if err := application.ValidateProviderChangeConsent(plan, consent); err == nil {
			t.Fatalf("incomplete double confirmation accepted: %q", answers)
		}
	}
	var out bytes.Buffer
	consent, err := readProviderApplyAcknowledgement(bufio.NewReader(strings.NewReader("yes\n")), &out, plan)
	if err != nil || !consent.DeleteData || !consent.First || !consent.Second || !strings.Contains(out.String(), "permanently delete the listed data") || strings.Count(out.String(), "[y/N]") != 1 {
		t.Fatalf("explicit acknowledgements: %#v %s %v", consent, out.String(), err)
	}
	if err := acknowledgeProviderImpact(context.Background(), nil, nil, plan, &consent); err != nil {
		t.Fatalf("missing alternative vetoed informed choice: %v", err)
	}
	plan.PlanID = "changed-effects"
	var failure *machine.Error
	if err := acknowledgeProviderImpact(context.Background(), nil, nil, plan, &consent); !errors.As(err, &failure) || failure.Code != machine.ErrorConflict {
		t.Fatalf("stale acknowledgement accepted: %v", err)
	}
}

func TestOrdinaryYesNeverAcknowledgesProviderEffects(t *testing.T) {
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, PlanID: "concrete-effects", Complete: false, ConfirmationsRequired: 2}
	ctx := withAssumeYes(machineNoninteractiveContext(context.Background()), true)
	var failure *machine.Error
	if err := acknowledgeProviderImpact(ctx, nil, new(bytes.Buffer), plan, nil); !errors.As(err, &failure) || failure.Code != machine.ErrorApprovalRequired {
		t.Fatalf("ordinary --yes inferred double acknowledgement: %v", err)
	}
	data, err := json.Marshal(machine.Classify(failure))
	if err != nil || !strings.Contains(string(data), `"details"`) || !strings.Contains(string(data), `"plan_id":"concrete-effects"`) {
		t.Fatalf("machine error lost concrete impact: %s %v", data, err)
	}
}

func TestProviderImpactConsentContextDoesNotGrantGenericApproval(t *testing.T) {
	consent := application.ProviderChangeConsent{PlanID: "reviewed", First: true}
	ctx := withProviderChangeConsent(context.Background(), &consent)
	consent.Second = true
	got := providerChangeConsentFromContext(ctx)
	if got == nil || got.Second || got.DeleteData {
		t.Fatalf("caller mutation changed acknowledgement scope: %#v", got)
	}
	got.Second = true
	if providerChangeConsentFromContext(ctx).Second {
		t.Fatal("reader mutation changed invocation consent")
	}
	if providerChangeConsentFromContext(context.Background()) != nil {
		t.Fatal("missing explicit consent was inferred")
	}
}

func TestCLIProviderConsentNeverLeaksIntoMachineServerAuthority(t *testing.T) {
	t.Setenv(providerChangeConsentEnv, `{"plan_id":"reviewed","first":true,"second":true,"delete_data":true}`)
	for _, command := range [][]string{{"mcp"}, {"serve"}} {
		got, err := invocationProviderChangeConsent(localCLIProviderConsentContext(context.Background(), command))
		if err != nil || got != nil {
			t.Fatalf("ambient CLI consent reached server %v: %#v %v", command, got, err)
		}
	}
	ctx := localCLIProviderConsentContext(context.Background(), []string{"app", "apply"})
	got, err := invocationProviderChangeConsent(ctx)
	if err != nil || got == nil || !got.First || !got.Second || !got.DeleteData {
		t.Fatalf("explicit local JSON transport: %#v %v", got, err)
	}
	if got, err := invocationProviderChangeConsent(withProviderChangeConsent(ctx, nil)); err != nil || got != nil {
		t.Fatal("missing machine request consent inherited CLI environment approval")
	}
	t.Setenv(providerChangeConsentEnv, `{"plan_id":"reviewed","first":true,"unexpected":true}`)
	if _, err := invocationProviderChangeConsent(ctx); err == nil {
		t.Fatal("unknown consent field silently accepted")
	}
}

func TestMaintenanceConsentRemainsSeparateFromDeploymentDecision(t *testing.T) {
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, PlanID: "owned-maintenance", ConfirmationsRequired: 2, Destructive: true}
	consent, err := readProviderImpactAcknowledgements(bufio.NewReader(strings.NewReader("yes\nyes\nno\n")), new(bytes.Buffer), plan)
	if err != nil {
		t.Fatal(err)
	}
	if consent.DeleteData || application.ValidateProviderChangeConsent(plan, consent) == nil {
		t.Fatal("deployment UX changed maintenance deletion consent")
	}
}

func TestProviderInstallationSummaryGroupsServicesWithoutInternalPaths(t *testing.T) {
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "dev-docker-1", PlanID: "internal-plan", Complete: true, ConfirmationsRequired: 2}
	for _, function := range []string{"database.sql", "database.sql", "cache.key-value"} {
		plan.Impacts = append(plan.Impacts, application.ProviderImpact{InstanceID: "internal-owner-uuid", Operation: "add", Functions: []application.FunctionConsequence{{Function: function, Outcome: "changed"}}, Materials: []application.MaterialConsequence{{Before: application.ProviderMaterial{ID: "internal-file", Location: &application.ResourceLocation{Reference: "/private/provider/config"}}, After: application.MaterialDisposition{Disposition: "retained"}}}})
	}
	var output bytes.Buffer
	if err := printProviderImpactForCLI(context.Background(), &output, plan); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if strings.Count(text, "Set up SQL") != 1 || !strings.Contains(text, "Set up Cache") {
		t.Fatalf("services not grouped: %s", text)
	}
	if strings.Contains(text, "internal-") || strings.Contains(text, "/private/") || strings.Count(text, "\n") > 10 {
		t.Fatalf("normal installation summary leaked implementation detail: %s", text)
	}
}

func TestDeploymentChangesDistinguishExistingMissingStoppedAndUnknown(t *testing.T) {
	plan := applicationLifecyclePlan{Footprint: application.Footprint{Requirements: []application.RequirementResolution{
		{Requirement: application.CapabilityRequirement{Kind: application.CapabilitySQL}, State: application.ProviderRunning},
		{Requirement: application.CapabilityRequirement{Kind: application.CapabilityKeyValue}, State: application.ProviderNewRequired},
		{Requirement: application.CapabilityRequirement{Kind: application.CapabilityObjectStorageS3}, State: application.ProviderStoppedOwned},
		{Requirement: application.CapabilityRequirement{Kind: application.CapabilityMessagingQueue}, State: application.ProviderUnverifiable},
	}}}
	var output bytes.Buffer
	printApplicationDeploymentChanges(&output, "dev-docker-1", plan)
	text := output.String()
	for _, want := range []string{"Reuse (already present): SQL", "Install: Cache", "Start existing services: Object storage", "Check and finish setup: Messaging", "Deploy / update the application"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing delta %q: %s", want, text)
		}
	}
	for _, bad := range []string{"Install: SQL", "Install: Messaging", "Services requested"} {
		if strings.Contains(text, bad) {
			t.Errorf("invented installation work: %s", text)
		}
	}
}
