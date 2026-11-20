package main

import (
	"context"
	"io"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

// runWithIOAsTestOperator keeps functional CLI tests on the same authenticated
// path as a real managed-environment invocation. Tests of unauthenticated
// denial call runWithIO directly so they continue to prove the fail-closed seam.
func runWithIOAsTestOperator(ctx context.Context, args []string, out, errOut io.Writer) error {
	return runWithIO(testOperatorContext(ctx), args, out, errOut)
}

func testOperatorContext(ctx context.Context) context.Context {
	if _, ok := operatorauth.PrincipalFromContext(ctx); ok {
		ctx = operatorauth.WithMembershipProviderIfAbsent(ctx, &readGrantingMembershipProvider{})
		return operatorauth.WithEnforcementIfAbsent(ctx)
	}
	principal := &identity.Principal{
		Issuer: "https://issuer.test", Subject: "cli-test-operator",
		IssuerVerified: true, Groups: []string{"editor"},
	}
	ctx = operatorauth.WithVerifiedPrincipal(ctx, principal)
	ctx = operatorauth.WithMembershipProviderIfAbsent(ctx, &readGrantingMembershipProvider{})
	return operatorauth.WithEnforcementIfAbsent(ctx)
}
