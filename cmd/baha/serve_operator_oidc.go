package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/controlplaneruntime"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

// serveEnrollmentAuthorityTarget pins both enrollment modes to the same owned
// installation. A per-application broker must never select a Core authority.
func serveEnrollmentAuthorityTarget(cfg controlplaneruntime.Config) string {
	if cfg.RuntimeAppName != "" || (!cfg.ConnectorEnrollmentEnabled && !cfg.WorkstationEnrollmentEnabled) {
		return ""
	}
	return strings.TrimSpace(cfg.ConnectorAuthorityTarget)
}

// The connector authority and the operator API must accept the same verified
// Keycloak issuer/client identity established by the Core's owner onboarding.
// This is derived from the protected target configuration, never from an
// installation/master realm or an invented identity provider.
func bindServeToOwnedOperatorOIDC(cfg *controlplaneruntime.Config) error {
	if cfg == nil {
		return errors.New("operator API configuration is unavailable")
	}
	if cfg.RuntimeAppName != "" || (!cfg.ConnectorEnrollmentEnabled && !cfg.WorkstationEnrollmentEnabled) {
		return nil
	}
	target := serveEnrollmentAuthorityTarget(*cfg)
	if target == "" {
		return errors.New("enrollment requires an owned authority target")
	}
	environment := strings.TrimSpace(os.Getenv("BASEHARBOR_OPERATOR_ENVIRONMENT"))
	if environment == "" {
		environment = "dev"
	}
	owned, found, err := resolveStoredOperatorAuthBoundary(target, environment)
	if err != nil {
		return fmt.Errorf("resolve Core operator OIDC boundary: %w", err)
	}
	if !found {
		// A workstation-only managed Core uses the existing local tenant,
		// protected database and verified client certificates. It must not
		// manufacture an operator/OIDC account just to pair a developer PC.
		if cfg.WorkstationEnrollmentEnabled && !cfg.ConnectorEnrollmentEnabled &&
			strings.TrimSpace(cfg.OIDCIssuer) == "" && len(cfg.OIDCAudiences) == 0 {
			return nil
		}
		return fmt.Errorf("operator OIDC is not configured for Core target %q and environment %q; run first-owner onboarding before enabling enrollment", target, environment)
	}
	return matchOwnedOperatorOIDC(cfg, owned)
}

func matchOwnedOperatorOIDC(cfg *controlplaneruntime.Config, owned operatorauth.Config) error {
	if cfg == nil {
		return errors.New("operator API configuration is unavailable")
	}
	if err := owned.Validate(); err != nil {
		return fmt.Errorf("configured operator OIDC is invalid: %w", err)
	}
	issuer := strings.TrimRight(strings.TrimSpace(cfg.OIDCIssuer), "/")
	if issuer != "" && issuer != owned.Issuer {
		return errors.New("operator API issuer disagrees with the verified operator realm; refusing to start a mismatched connector authority")
	}
	if len(cfg.OIDCAudiences) > 0 {
		matched := false
		for _, value := range cfg.OIDCAudiences {
			if strings.TrimSpace(value) == owned.ClientID {
				matched = true
				break
			}
		}
		if !matched {
			return errors.New("operator API audiences do not include the configured operator OIDC client")
		}
	} else {
		cfg.OIDCAudiences = []string{owned.ClientID}
	}
	cfg.OIDCIssuer = owned.Issuer
	return nil
}
