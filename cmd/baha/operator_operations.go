package main

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"strings"
)

type operatorIdentityResult struct {
	Target      string           `json:"target"`
	Environment string           `json:"environment"`
	Actor       machine.ActorRef `json:"actor"`
}

func inspectOperatorIdentity(ctx context.Context, environment string) (operatorIdentityResult, error) {
	environment = strings.ToLower(strings.TrimSpace(environment))
	if environment == "" {
		return operatorIdentityResult{}, usageError("operator identity requires an environment", "Specify the Target/Environment authentication boundary.")
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return operatorIdentityResult{}, err
	}
	result := operatorIdentityResult{Target: target.Name, Environment: environment, Actor: machine.ActorRef{Mode: "trusted-local", Subject: "local-operator"}}
	if !operatorauth.ManagedEnvironment(environment) {
		return result, nil
	}
	cfg, found, err := resolveStoredOperatorAuthBoundary(target.Name, environment)
	if err != nil {
		return result, err
	}
	if !found {
		return result, machine.NewError(machine.ErrorAuthenticationFailed, "Operator authentication is not configured.", "Configure operator OIDC for the selected Target/environment and log in.", false)
	}
	principal, err := operatorauth.VerifySession(ctx, target.Name, environment, cfg)
	if err != nil {
		return result, machine.Wrap(machine.ErrorAuthenticationFailed, err, "Run baha login for the selected Target/environment.", false)
	}
	ctx = operatorauth.WithVerifiedPrincipal(ctx, principal)
	if err := authorizeMCPOperation(ctx, "operator.identity", target.Name, environment, "", ""); err != nil {
		return result, err
	}
	result.Actor = machine.ActorRef{Mode: "authenticated", Issuer: principal.Issuer, Subject: principal.Subject, Assurance: principal.Assurance, Methods: principal.Methods}
	return result, nil
}
