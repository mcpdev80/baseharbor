package operatorauth

import (
	"context"

	"github.com/mcpdev80/baseharbor/internal/identity"
)

type enforcementState struct {
	principal *identity.Principal
	decision  *AuthorizationDecision
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

func setAuthorizationDecision(ctx context.Context, decision AuthorizationDecision) {
	if state, ok := ctx.Value(enforcementKey{}).(*enforcementState); ok && state != nil {
		copy := decision
		state.decision = &copy
	}
}

func AuthorizationDecisionFromContext(ctx context.Context) (AuthorizationDecision, bool) {
	state, ok := ctx.Value(enforcementKey{}).(*enforcementState)
	if !ok || state == nil || state.decision == nil {
		return AuthorizationDecision{}, false
	}
	return *state.decision, true
}

func WithVerifiedPrincipal(ctx context.Context, principal *identity.Principal) context.Context {
	ctx = WithEnforcement(ctx)
	setPrincipal(ctx, principal)
	return ctx
}

type membershipProviderKey struct{}

// WithMembershipProvider installs the bounded MembershipProvider that maps a
// verified operator's issuer+subject/group/app-role claims onto concrete
// tenant memberships. It is an optional context value; without it a
// claim-based resolution fails closed (ErrClaimsNotVerified).
func WithMembershipProvider(ctx context.Context, provider MembershipProvider) context.Context {
	return context.WithValue(ctx, membershipProviderKey{}, provider)
}

// WithMembershipProviderIfAbsent installs provider only when the caller has
// not already supplied one. Production composition can add its database-backed
// resolver without replacing an explicitly scoped provider.
func WithMembershipProviderIfAbsent(ctx context.Context, provider MembershipProvider) context.Context {
	if membershipProviderFromContext(ctx) != nil || provider == nil {
		return ctx
	}
	return WithMembershipProvider(ctx, provider)
}

func membershipProviderFromContext(ctx context.Context) MembershipProvider {
	provider, ok := ctx.Value(membershipProviderKey{}).(MembershipProvider)
	if !ok || provider == nil {
		return nil
	}
	return provider
}
