package contracts_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/contracts"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// All machine interfaces serialize this same domain output; presentation is not
// allowed to reinterpret consequences, consent or data disposition.
func TestProviderImpactMachineSchemasAndSharedFixtures(t *testing.T) {
	schema := contractSchema(t, "provider-impact/v1/impact.schema.json")
	consentSchema := contractSchema(t, "provider-impact/v1/consent.schema.json")
	resultSchema := contractSchema(t, "provider-impact/v1/result.schema.json")
	unionSchema := contractSchema(t, "provider-plan/v1/plan.schema.json")
	raw, err := os.ReadFile("../internal/application/testdata/v0425-provider-impact.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name          string                                `json:"name"`
		Snapshot      application.ResolutionSnapshot        `json:"snapshot"`
		Observation   application.ProviderImpactObservation `json:"observation"`
		Requests      []application.ProviderChangeRequest   `json:"requests"`
		Manifest      string                                `json:"manifest"`
		ApplicationID string                                `json:"application_id"`
		Complete      bool                                  `json:"expect_complete"`
		Destructive   bool                                  `json:"expect_destructive"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		t.Run(f.Name, func(t *testing.T) {
			manifest, err := application.ParseYAML(f.Manifest)
			if err != nil {
				t.Fatal(err)
			}
			manifest.ApplicationID = f.ApplicationID
			union, err := application.ResolveProviderPlan(f.Snapshot, []application.ConsumerIntent{{Key: "app/one", Active: true, Manifest: manifest}}, true)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := application.DescribeProviderChanges(f.Snapshot, union, f.Requests, f.Observation)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Complete != f.Complete || plan.Destructive != f.Destructive {
				t.Fatalf("%+v", plan)
			}
			value := jsonValue(t, plan)
			if err := schema.Validate(value); err != nil {
				t.Fatal(err)
			}
			failure := &machine.Error{Code: machine.ErrorPolicyDenied, Message: "Provider changes need current acknowledgements", CauseCode: "provider_change_consent_required", Details: plan}
			wire, err := json.Marshal(machine.ResultError(failure))
			if err != nil {
				t.Fatal(err)
			}
			if err := contracts.ValidateMachineControlRecord("error_result", wire); err != nil {
				t.Fatalf("HTTP error envelope rejects the actual provider impact plan: %v", err)
			}
			now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
			actor := machine.ActorRef{Mode: "operator", Subject: "fixture"}
			context := machine.OperationContext{Target: plan.Target, Environment: manifest.Environment, Application: manifest.Name}
			execution := machine.Execution{ContractVersion: "v1", ExecutionID: "exec_0123456789abcdef0123456789abcdef", OperationID: "apply", Actor: actor, Context: context, State: machine.ExecutionFailed, StartedAt: &now, FinishedAt: &now, Error: failure}
			event := machine.MachineEvent{ContractVersion: "v1", Sequence: 1, OccurredAt: now, Kind: machine.EventOperationFailed, ExecutionID: execution.ExecutionID, OperationID: execution.OperationID, Actor: &actor, Context: &context, State: execution.State, Error: failure}
			for record, value := range map[string]any{"execution": execution, "event": event} {
				wire, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				if err := contracts.ValidateMachineControlRecord(record, wire); err != nil {
					t.Fatalf("%s rejects the actual provider impact plan: %v", record, err)
				}
			}
			if err := consentSchema.Validate(jsonValue(t, application.ProviderChangeConsent{PlanID: plan.PlanID, First: true, Second: true, DeleteData: plan.Destructive})); err != nil {
				t.Fatal(err)
			}
			// Missing alternatives are executable after the applicable TWO stages.
			if err := application.ValidateProviderChangeConsent(plan, application.ProviderChangeConsent{PlanID: plan.PlanID, First: true, Second: true, DeleteData: plan.Destructive}); err != nil {
				t.Fatal(err)
			}
			union.Impact = &plan
			if err := unionSchema.Validate(jsonValue(t, union)); err != nil {
				t.Fatal(err)
			}
			result, err := application.RecordProviderChangeResult(plan, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := resultSchema.Validate(jsonValue(t, result)); err != nil {
				t.Fatal(err)
			}
			value["secret_value"] = "must never be accepted"
			if err := schema.Validate(value); err == nil {
				t.Fatal("machine contract accepted secret contents")
			}
			delete(value, "secret_value")
			if !plan.Complete || plan.Destructive {
				value["confirmations_required"] = float64(1)
				if err := schema.Validate(value); err == nil {
					t.Fatal("schema allowed one-stage unknown/destructive consent")
				}
			}
		})
	}
}

func jsonValue(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
