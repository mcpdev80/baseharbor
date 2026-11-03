package serviceaccess

import (
	"context"

	"github.com/mcpdev80/baseharbor/internal/installauthority"
)

// authority.go holds the minimal integration trammels that connect the
// v0.4.27 authority contract (see coreinstallation/authority.go and ADR 0021)
// to the real production authority boundary: the provider-neutral
// serviceaccess.Issuer consumed by the control plane.
//
// These helpers do NOT change any production ServiceIssuer / LocalIssuer
// behavior. They only bridge the production issuer into the stdlib-only
// installauthority leaf that coreinstallation depends on, so coreinstallation
// never imports serviceaccess for authority purposes. The production issuer
// already satisfies Issuer; the adapter below projects just the read-only
// decision surface (Status/Revoke) that the leaf's decision helpers consume.

// IssuerAdapter adapts a production Issuer to the installauthority.Issuer
// decision surface consumed by coreinstallation. It exposes only Status and
// Revoke (the read-only foreign/revoked boundary); Issue/Renew/TrustBundle are
// intentionally not projected so the management-connection decision can never
// mutate issuer state.
type IssuerAdapter struct {
	real Issuer
}

// NewIssuerAdapter wraps a production issuer in the leaf decision adapter.
func NewIssuerAdapter(real Issuer) *IssuerAdapter {
	return &IssuerAdapter{real: real}
}

// Status projects the production readiness onto the leaf IssuerStatus.
func (a *IssuerAdapter) Status(ctx context.Context) (installauthority.IssuerStatus, error) {
	if a == nil || a.real == nil {
		return installauthority.IssuerStatus{}, installauthority.ErrMissingAuthority
	}
	status, err := a.real.Status(ctx)
	if err != nil {
		return installauthority.IssuerStatus{}, err
	}
	return installauthority.IssuerStatus{
		IssuerReference: status.IssuerReference,
		Ready:           status.Ready,
	}, nil
}

// Revoke projects the read-only foreign/revoked check onto the leaf boundary.
func (a *IssuerAdapter) Revoke(ctx context.Context, serial string) error {
	return a.real.Revoke(ctx, serial)
}

// LeafStatus is a thin helper returning the leaf IssuerStatus without a wrapper
// value, for composition roots that call the decision helpers directly.
func LeafStatus(ctx context.Context, real Issuer) (installauthority.IssuerStatus, error) {
	return NewIssuerAdapter(real).Status(ctx)
}
