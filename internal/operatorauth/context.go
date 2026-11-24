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

// skipResolveBoundaryKey, when present, tells ensureOperatorAuthForBoundary to
// defer the interactive operator-auth boundary until the caller re-authorizes
// explicitly (after caller-error usage validation has run). It never weakens
// the enforced boundary; it only reorders it so usage failures surface before
// an authorization failure.
type skipResolveBoundaryKey struct{}

// WithSkipResolveOperatorBoundary marks ctx so the operator-auth boundary does
// not fire during application resolution. Callers must re-authorize the actual
// runtime operation before anything runs.
func WithSkipResolveOperatorBoundary(ctx context.Context) context.Context {
	return context.WithValue(ctx, skipResolveBoundaryKey{}, true)
}

// ShouldSkipResolveOperatorBoundary reports whether ctx carries the deferred
// operator-auth boundary marker.
func ShouldSkipResolveOperatorBoundary(ctx context.Context) bool {
	v, ok := ctx.Value(skipResolveBoundaryKey{}).(bool)
	return ok && v
}

// ClearResolveOperatorBoundary returns a ctx without the deferred-boundary
// marker so the next operator-auth boundary check re-enforces the boundary.
func ClearResolveOperatorBoundary(ctx context.Context) context.Context {
	if !ShouldSkipResolveOperatorBoundary(ctx) {
		return ctx
	}
	return context.WithValue(ctx, skipResolveBoundaryKey{}, false)
}

func WithEnforcement(ctx context.Context) context.Context {
	return context.WithValue(ctx, enforcementKey{}, &enforcementState{})
}

// WithEnforcementIfAbsent preserves enforcement context supplied by an
// authenticated caller. Replacing it would discard its verified principal
// and any authorization decision already bound to the request.
func WithEnforcementIfAbsent(ctx context.Context) context.Context {
	if EnforcementEnabled(ctx) {
		return ctx
	}
	return WithEnforcement(ctx)
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

// WithAuthorizationDecision installs a stored AuthorizationDecision in the
// enforcement context. It is used by tests to model a verified principal that
// also carries a machine authorization decision, and by callers that have
// resolved an explicit decision.
func WithAuthorizationDecision(ctx context.Context, decision AuthorizationDecision) context.Context {
	ctx = WithEnforcement(ctx)
	setAuthorizationDecision(ctx, decision)
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
