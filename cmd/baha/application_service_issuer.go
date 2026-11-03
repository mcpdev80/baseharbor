package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mcpdev80/baseharbor/internal/installauthority"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// Service access does not imply a management installation. Reuse a selected
// installation's existing issuer; otherwise realize the same issuer contract
// locally without adding a SQL, Secrets or Identity runtime service.
func resolveApplicationServiceIssuer(ctx context.Context, resolved resolvedApplication, workload bhruntime.RuntimeProvider) (serviceaccess.Issuer, error) {
	selected := withTargetOverride(ctx, resolved.Target.Name)
	target, remote, err := applicationCoreTarget(selected)
	if err != nil {
		return nil, err
	}
	current, err := effectiveTarget(selected)
	if err != nil {
		return nil, err
	}
	if current != resolved.Target {
		return nil, errors.New("application Target binding changed during execution")
	}
	dir := filepath.Join(resolved.TargetStateRoot, "service-access", "issuer")
	if !remote {
		if _, err := os.Lstat(dir); err == nil {
			if err := serviceaccess.CheckLocalIssuer(dir); err != nil {
				return nil, err
			}
			return &serviceaccess.LocalIssuer{Dir: dir}, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(root); err == nil || remote {
		provider, files, err := resolveApplicationCoreRuntime(ctx, resolved, workload)
		if err != nil {
			return nil, err
		}
		issuer := openbao.NewServiceIssuer(provider, files)
		status, err := issuer.Status(ctx)
		if err != nil {
			return nil, err
		}
		if !status.Ready {
			return nil, unavailableApplicationCoreRuntime()
		}
		return issuer, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := serviceaccess.CheckLocalIssuer(dir); err != nil {
		return nil, err
	}
	return &serviceaccess.LocalIssuer{Dir: dir}, nil
}

// Only the authorized mutation phase initializes new authority. All plan and
// preflight callers above remain read-only and preserve damaged state.
func materializeApplicationServiceIssuer(issuer serviceaccess.Issuer) error {
	if local, ok := issuer.(*serviceaccess.LocalIssuer); ok {
		_, err := serviceaccess.EnsureLocalIssuer(local.Dir)
		return err
	}
	return nil
}

// resolveManagementConnectionIssuer is the single authorized production
// selection point for the management-connection authority. It returns the
// verified serviceaccess.Issuer for the selected installation and proves,
// through the read-only installauthority.Fresh boundary, that the observed
// connection is live, current and not foreign or revoked before the issuer is
// returned to a caller. The mutating openbao/LocalIssuer Revoke action never
// runs on this verification path; a missing, foreign, expired or revoked
// connection selects no issuer (fails closed).
func resolveManagementConnectionIssuer(ctx context.Context, resolved resolvedApplication, workload bhruntime.RuntimeProvider) (serviceaccess.Issuer, error) {
	issuer, err := resolveApplicationServiceIssuer(ctx, resolved, workload)
	if err != nil {
		return nil, err
	}
	observed, err := observedThroughBoundary(ctx, issuer)
	if err != nil {
		return nil, err
	}
	if err := installauthority.Fresh(ctx, issuer, observed, observed.Identity); err != nil {
		return nil, fmt.Errorf("management connection not verified as a live, current, non-revoked authority: %w", err)
	}
	return issuer, nil
}

// observedThroughBoundary issues a fresh leaf and binds the observed
// CertificateStatus (identity = issuer reference, freshness bound = leaf
// ExpiresAt, foreign/revoked target = leaf Serial) so the same real selection
// is verified over one consistent observation.
func observedThroughBoundary(ctx context.Context, issuer serviceaccess.Issuer) (serviceaccess.CertificateStatus, error) {
	leaf, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{CommonName: "management-connection", TTL: 90 * 24 * time.Hour})
	if err != nil {
		return serviceaccess.CertificateStatus{}, err
	}
	trust, err := issuer.TrustBundle(ctx)
	if err != nil {
		return serviceaccess.CertificateStatus{}, err
	}
	return serviceaccess.CertificateStatus{Identity: trust.IssuerReference, IssuedCertificate: leaf}, nil
}
