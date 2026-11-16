package contracts_test

import (
	"github.com/mcpdev80/baseharbor/internal/application"
	"testing"
)

func TestProviderImpactScopeOfflineMachineContract(t *testing.T) {
	schema := contractSchema(t, "provider-impact/v1/scope.schema.json")
	resultSchema := contractSchema(t, "provider-impact/v1/scope-result.schema.json")
	owner := application.ProviderScopeOwner{Owner: "existing/global-owner", Reference: "/protected/external-providers.json", Subject: "company-provider", Revision: "observed-content/1"}
	for _, complete := range []bool{true, false} {
		scope, err := application.ComposeProviderImpactScope(owner, nil, complete)
		if err != nil {
			t.Fatal(err)
		}
		value := jsonValue(t, scope)
		if err := schema.Validate(value); err != nil {
			t.Fatal(err)
		}
		result, err := application.RecordProviderImpactScopeResult(scope, application.ProviderScopeOwnerOutcome{Owner: owner, Disposition: "retained"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := resultSchema.Validate(jsonValue(t, result)); err != nil {
			t.Fatal(err)
		}
		value["secret_value"] = "never allowed"
		if err := schema.Validate(value); err == nil {
			t.Fatal("scope schema accepted secret values")
		}
		delete(value, "secret_value")
		if !complete {
			value["confirmations_required"] = float64(1)
			if err := schema.Validate(value); err == nil {
				t.Fatal("unknown scope schema accepted one acknowledgement")
			}
		}
	}
}
