package operatorauth

import (
	"context"

	"github.com/mcpdev80/baseharbor/internal/identity"
)

type enforcementState struct {
	principal *identity.Principal
}

type enforcementKey struct{}

func WithEnforcement(ctx context.Context) context.Context {
	return context.WithValue(ctx, enforcementKey{}, &enforcementState{})
}

func EnforcementEnabled(ctx context.Context) bool {
	_, ok := ctx.Value(enforcementKey{}).(*enforcementState)
	return ok
}

func setPrincipal(ctx context.Context, principal *identity.Principal) {
	if state, ok := ctx.Value(enforcementKey{}).(*enforcementState); ok {
		state.principal = principal
	}
}

func PrincipalFromContext(ctx context.Context) (*identity.Principal, bool) {
	state, ok := ctx.Value(enforcementKey{}).(*enforcementState)
	if !ok || state == nil || state.principal == nil {
		return nil, false
	}
	return state.principal, true
}
