package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

type operatorAuthObservation struct {
	Mode      string   `json:"mode"`
	Provider  string   `json:"provider,omitempty"`
	Status    string   `json:"status"`
	Session   string   `json:"session"`
	Principal string   `json:"principal,omitempty"`
	Issuer    string   `json:"issuer,omitempty"`
	Assurance string   `json:"assurance,omitempty"`
	Methods   []string `json:"authentication_methods,omitempty"`
	Detail    string   `json:"detail,omitempty"`
}

func collectOperatorAuthObservation(ctx context.Context, target, environment string) operatorAuthObservation {
	if !operatorauth.ManagedEnvironment(environment) {
		return operatorAuthObservation{
			Mode: "trusted-local", Status: "READY", Session: "trusted-local",
			Principal: "local-operator",
		}
	}
	cfg, found, err := resolveStoredOperatorAuthBoundary(target, environment)
	if err != nil {
		return operatorAuthObservation{
			Mode: "oidc", Status: "DEGRADED", Session: "unknown", Detail: err.Error(),
		}
	}
	if !found {
		return operatorAuthObservation{
			Mode: "oidc", Status: "NOT_CONFIGURED", Session: "none",
			Detail: "operator authentication is not configured for this Target/Environment boundary",
		}
	}
	provider := strings.TrimSpace(cfg.Provider)
	switch provider {
	case "managed-keycloak":
		provider = "managed/shared"
	case "external-oidc":
		provider = "external"
	}
	result := operatorAuthObservation{
		Mode: "oidc", Provider: provider, Status: "READY", Session: "unauthenticated",
		Issuer: strings.TrimRight(strings.TrimSpace(cfg.Issuer), "/"),
	}
	session, err := operatorauth.ObserveSession(target, environment, cfg)
	if err != nil {
		result.Status = "DEGRADED"
		result.Session = "unknown"
		result.Detail = err.Error()
		return result
	}
	if session.Valid {
		result.Session = "authenticated"
		result.Principal = session.Subject
		result.Issuer = session.Issuer
	} else if session.Present {
		result.Session = "expired"
	}
	if principal, ok := operatorauth.PrincipalFromContext(ctx); ok {
		result.Session = "authenticated"
		result.Principal = principal.Subject
		result.Issuer = principal.Issuer
		result.Assurance = principal.Assurance
		result.Methods = append([]string(nil), principal.Methods...)
	}
	return result
}

func renderOperatorAuthObservation(out interface{ Write([]byte) (int, error) }, observation operatorAuthObservation) {
	fmt.Fprintln(out, "Operator authentication")
	fmt.Fprintf(out, "  mode       %s\n", observation.Mode)
	if observation.Provider != "" {
		fmt.Fprintf(out, "  provider   %s\n", observation.Provider)
	}
	fmt.Fprintf(out, "  status     %s\n", observation.Status)
	fmt.Fprintf(out, "  session    %s\n", observation.Session)
	if observation.Principal != "" {
		fmt.Fprintf(out, "  principal  %s\n", observation.Principal)
	}
	if observation.Issuer != "" {
		fmt.Fprintf(out, "  issuer     %s\n", observation.Issuer)
	}
	if observation.Detail != "" {
		fmt.Fprintf(out, "  detail     %s\n", observation.Detail)
	}
}
