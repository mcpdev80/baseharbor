package contracts_test

import (
	"encoding/json"
	"github.com/mcpdev80/baseharbor/internal/application"
	"os"
	"testing"
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
