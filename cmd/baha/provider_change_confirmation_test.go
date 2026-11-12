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
	for _, want := range []string{"plan-current", "2 confirmation", "Permanent data deletion: requested", "inventory is incomplete", "REMOVE-RUNTIME", "postgres-owned", "database.sql -> unavailable", "required; consumer: demo/dev", "database-volume -> delete", "volume-owned", "permanently removed", "sessions: active -> invalidated"} {
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
	for _, answers := range []string{"yes\n", "yes\nno\n", "no\nyes\n", "\nyes\n", "yes\nyes\n", "yes\nyes\nno\n", "yes\nyes\n\n"} {
		consent, err := readProviderImpactAcknowledgements(bufio.NewReader(strings.NewReader(answers)), new(bytes.Buffer), plan)
		if err != nil {
			t.Fatal(err)
		}
		if err := application.ValidateProviderChangeConsent(plan, consent); err == nil {
			t.Fatalf("incomplete double confirmation accepted: %q", answers)
		}
	}
	var out bytes.Buffer
	consent, err := readProviderImpactAcknowledgements(bufio.NewReader(strings.NewReader("yes\nyes\nyes\n")), &out, plan)
	if err != nil || !consent.DeleteData || !strings.Contains(out.String(), "Achtung") || !strings.Contains(out.String(), "2/2") || !strings.Contains(out.String(), "separately authorize") {
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
