package operatorauth

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

func TestVerifiedLocalRuntimeAuthorityIsOperationTargetEnvironmentBound(t *testing.T) {
	apply, _ := machine.OperationByID("apply")
	request := AuthorizationRequest{Operation: apply, Context: OperationContext{Target: "owned", Environment: "prod", Application: "app"}}
	ctx := WithVerifiedLocalRuntimeAuthority(WithEnforcement(context.Background()), LocalRuntimeAuthority{Operation: "apply", Target: "owned", Environment: "prod", Runtime: "docker"})
	decision, err := AuthorizeMachineOperation(ctx, request)
	if err != nil || !decision.Allowed || decision.Actor.Mode != "local-runtime" || decision.Actor.Issuer != "" || decision.ConfirmationRequired != apply.ConfirmationRequired || decision.PolicyRequired != apply.PolicyRequired {
		t.Fatalf("local runtime evidence changed policy or claimed an OIDC principal: %+v %v", decision, err)
	}
	if _, principal := PrincipalFromContext(ctx); principal {
		t.Fatal("local runtime authority manufactured an OIDC principal")
	}
	for _, change := range []string{"target", "environment", "operation", "management", "stripped", "tenant"} {
		t.Run(change, func(t *testing.T) {
			r := request
			candidate := ctx
			switch change {
			case "target":
				r.Context.Target = "foreign"
			case "environment":
				r.Context.Environment = "test"
			case "operation":
				r.Operation, _ = machine.OperationByID("destroy")
			case "management":
				r.Operation, _ = machine.OperationByID("control-plane.up")
				candidate = WithVerifiedLocalRuntimeAuthority(ctx, LocalRuntimeAuthority{Operation: "control-plane.up", Target: "owned", Environment: "prod", Runtime: "docker"})
			case "stripped":
				candidate = WithoutLocalRuntimeAuthority(ctx)
			case "tenant":
				candidate = tenancy.WithContext(ctx, &tenancy.Context{TenantID: "tenant", ExternalIdentityID: "member"})
			}
			if decision, err := AuthorizeMachineOperation(candidate, r); err == nil || decision.Allowed {
				t.Fatal("scoped local authority escaped its verified operation/Target/environment or tenant boundary")
			}
		})
	}
}
