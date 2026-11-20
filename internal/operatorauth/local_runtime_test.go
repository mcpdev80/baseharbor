package operatorauth

import (
	"context"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

func TestLocalRuntimeGrantIsOperationTargetEnvironmentBoundAndSingleUse(t *testing.T) {
	apply, _ := machine.OperationByID("apply")
	const environmentID = "5b09b7a8-9380-43cc-8000-000000000001"
	const applicationID = "5b09b7a8-9380-43cc-8000-000000000002"
	request := AuthorizationRequest{Operation: apply, Context: OperationContext{Target: "owned", Environment: "prod", Application: "app"}, EnvironmentID: environmentID, TargetEnvironmentID: environmentID, ApplicationID: applicationID}
	grant, err := CheckLocalRuntimeGrant(apply, environmentID, applicationID, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithEnforcement(context.Background())
	decision, err := AuthorizeMachineOperationWithLocalGrant(ctx, request, grant)
	if err != nil || !decision.Allowed || decision.Actor.Mode != "local-runtime" || decision.Actor.Issuer != "" || decision.ConfirmationRequired != apply.ConfirmationRequired || decision.PolicyRequired != apply.PolicyRequired {
		t.Fatalf("local runtime evidence changed policy or claimed an OIDC principal: %+v %v", decision, err)
	}
	if decision, err := AuthorizeMachineOperationWithLocalGrant(ctx, request, grant); err == nil || decision.Allowed {
		t.Fatal("a local runtime grant authorized a second policy check")
	}
	if _, principal := PrincipalFromContext(ctx); principal {
		t.Fatal("local runtime authority manufactured an OIDC principal")
	}
	for _, change := range []string{"target", "environment", "application", "application-dev", "operation", "missing", "management", "tenant"} {
		t.Run(change, func(t *testing.T) {
			candidateGrant, issueErr := CheckLocalRuntimeGrant(apply, environmentID, applicationID, func() error { return nil })
			if issueErr != nil {
				t.Fatal(issueErr)
			}
			r := request
			candidate := candidateGrant
			switch change {
			case "target":
				r.TargetEnvironmentID = "foreign-env"
			case "environment":
				r.EnvironmentID = "foreign-env"
			case "application":
				r.ApplicationID = "foreign-app"
			case "application-dev":
				r.Context.Environment = "dev"
				r.ApplicationID = "foreign-app"
			case "operation":
				r.Operation, _ = machine.OperationByID("destroy")
			case "management":
				r.Operation, _ = machine.OperationByID("control-plane.up")
			case "missing":
				candidate = nil
			case "tenant":
				ctx := tenancy.WithContext(ctx, &tenancy.Context{TenantID: "tenant", ExternalIdentityID: "member"})
				if decision, err := AuthorizeMachineOperationWithLocalGrant(ctx, r, candidate); err == nil || decision.Allowed {
					t.Fatal("tenant-scoped operation was authorized by a local grant")
				}
				return
			}
			if decision, err := AuthorizeMachineOperationWithLocalGrant(ctx, r, candidate); err == nil || decision.Allowed {
				t.Fatal("scoped local authority escaped its verified operation/Target/environment or tenant boundary")
			}
		})
	}
}

func TestCheckLocalRuntimeGrantFailsClosedAndUsesMonotonicNonce(t *testing.T) {
	apply, _ := machine.OperationByID("apply")
	first, err := CheckLocalRuntimeGrant(apply, "5b09b7a8-9380-43cc-8000-000000000001", "5b09b7a8-9380-43cc-8000-000000000002", func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	second, err := CheckLocalRuntimeGrant(apply, "5b09b7a8-9380-43cc-8000-000000000001", "5b09b7a8-9380-43cc-8000-000000000002", func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	firstNonce := first.(*localOperationGrant).nonce
	secondNonce := second.(*localOperationGrant).nonce
	if firstNonce == 0 || secondNonce <= firstNonce {
		t.Fatalf("grant nonce did not increase monotonically: first=%d second=%d", firstNonce, secondNonce)
	}
	for _, tc := range []struct {
		name          string
		operation     machine.Operation
		environmentID string
		applicationID string
		check         func() error
	}{
		{name: "unknown-operation", operation: machine.Operation{ID: "not.registered"}, environmentID: "5b09b7a8-9380-43cc-8000-000000000001", applicationID: "5b09b7a8-9380-43cc-8000-000000000002", check: func() error { return nil }},
		{name: "invalid-environment-uid", operation: apply, environmentID: "prod", applicationID: "5b09b7a8-9380-43cc-8000-000000000002", check: func() error { return nil }},
		{name: "invalid-application-uid", operation: apply, environmentID: "5b09b7a8-9380-43cc-8000-000000000001", applicationID: "demo", check: func() error { return nil }},
		{name: "runtime-check-failed", operation: apply, environmentID: "5b09b7a8-9380-43cc-8000-000000000001", applicationID: "5b09b7a8-9380-43cc-8000-000000000002", check: func() error { return errors.New("runtime mismatch") }},
		{name: "missing-check", operation: apply, environmentID: "5b09b7a8-9380-43cc-8000-000000000001", applicationID: "5b09b7a8-9380-43cc-8000-000000000002"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if grant, err := CheckLocalRuntimeGrant(tc.operation, tc.environmentID, tc.applicationID, tc.check); err == nil || grant != nil {
				t.Fatalf("invalid local check issued a grant: grant=%v err=%v", grant, err)
			}
		})
	}
}
