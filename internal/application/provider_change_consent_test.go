package application

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func TestAlternativeLessDeletionRequiresTwoPlanBoundStages(t *testing.T) {
	raw, err := os.ReadFile("testdata/v0425-provider-impact-consent.json")
	if err != nil {
		t.Fatal(err)
	}
	var plan ProviderImpactPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		consent ProviderChangeConsent
		code    string
	}{
		{ProviderChangeConsent{PlanID: "old", First: true, Second: true, DeleteData: true}, "stale-impact-consent"},
		{ProviderChangeConsent{PlanID: plan.PlanID, First: true, DeleteData: true}, "impact-confirmation-required"},
		{ProviderChangeConsent{PlanID: plan.PlanID, Second: true, DeleteData: true}, "impact-confirmation-required"},
		{ProviderChangeConsent{PlanID: plan.PlanID, First: true, Second: true}, "data-deletion-consent-required"},
		{ProviderChangeConsent{PlanID: plan.PlanID, First: true, Second: true, DeleteData: true}, ""},
	} {
		err := ValidateProviderChangeConsent(plan, tc.consent)
		if tc.code == "" {
			if err != nil {
				t.Fatal(err)
			}
			continue
		}
		var typed *ProviderChangeError
		if !errors.As(err, &typed) || typed.Code != tc.code {
			t.Fatalf("%v; expected %s", err, tc.code)
		}
	}
}
