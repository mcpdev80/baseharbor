package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	IdentityBindingName = "identity"
	IdentityWorkloadClientSecretFile = "/run/baseharbor/service-bindings/identity/client-secret"
)

type IdentityDiscovery struct {
	Issuer                string
	AuthorizationEndpoint string
	TokenEndpoint         string
	UserinfoEndpoint      string
	JWKSURI               string
	EndSessionEndpoint    string
}

func MaterializeIdentityBinding(m Manifest, files RuntimeFiles, provider string, discovery IdentityDiscovery, clientID, clientSecret string) error {
	if !m.Services.Identity {
		return fmt.Errorf("identity binding requires services.identity enabled")
	}
	if err := validateIdentityDiscovery(discovery); err != nil {
		return err
	}
	provider = strings.TrimSpace(provider)
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)
	if provider == "" || clientID == "" {
		return fmt.Errorf("identity binding provider and client id are required")
	}

	binding := filepath.Join(files.Bindings, IdentityBindingName)
	if err := os.MkdirAll(binding, 0o700); err != nil {
		return fmt.Errorf("create identity binding: %w", err)
	}
	entries := map[string]string{
		"type":                        "oidc",
		"provider":                    provider,
		"uri":                         discovery.Issuer,
		"oidc.issuer":                 discovery.Issuer,
		"oidc.authorization-endpoint": discovery.AuthorizationEndpoint,
		"oidc.token-endpoint":         discovery.TokenEndpoint,
		"oidc.userinfo-endpoint":      discovery.UserinfoEndpoint,
		"oidc.jwks-uri":               discovery.JWKSURI,
		"oidc.client-id":              clientID,
	}
	if discovery.EndSessionEndpoint != "" {
		entries["oidc.end-session-endpoint"] = discovery.EndSessionEndpoint
	}
	if len(m.Identity.Scopes) > 0 {
		entries["oidc.scopes"] = strings.Join(m.Identity.Scopes, " ")
	}
	if len(m.Identity.Claims) > 0 {
		entries["oidc.claims"] = strings.Join(m.Identity.Claims, " ")
	}
	if clientSecret != "" {
		entries["client-secret"] = clientSecret
	}
	for name, value := range entries {
		if err := capabilityBindingEntryName(name); err != nil {
			return err
		}
		if err := writeOwnerOnlyFile(filepath.Join(binding, name), []byte(value+"\n")); err != nil {
			return fmt.Errorf("write identity binding %s: %w", name, err)
		}
	}

	workload := filepath.Join(workloadServiceBindingProjectionDir(files), IdentityBindingName)
	if err := os.MkdirAll(workload, 0o755); err != nil {
		return fmt.Errorf("create workload identity binding: %w", err)
	}
	for name, value := range entries {
		path := filepath.Join(workload, name)
		if err := os.WriteFile(path, []byte(value+"\n"), 0o444); err != nil {
			return fmt.Errorf("project workload identity binding %s: %w", name, err)
		}
		if err := os.Chmod(path, 0o444); err != nil {
			return err
		}
	}

	values, err := loadApplicationEnvValues(files.ApplicationEnv)
	if err != nil {
		return err
	}
	values["OIDC_ISSUER"] = discovery.Issuer
	values["OIDC_CLIENT_ID"] = clientID
	if len(m.Identity.Scopes) > 0 {
		values["OIDC_SCOPES"] = strings.Join(m.Identity.Scopes, " ")
	}
	if clientSecret != "" {
		values["OIDC_CLIENT_SECRET_FILE"] = filepath.Join(binding, "client-secret")
	}
	if err := writeApplicationEnvValues(files.ApplicationEnv, values); err != nil {
		return err
	}
	return nil
}

func VerifyIdentityBinding(m Manifest, files RuntimeFiles) error {
	if !m.Services.Identity {
		return nil
	}
	dir := filepath.Join(files.Bindings, IdentityBindingName)
	required := []string{
		"type", "provider", "uri", "oidc.issuer", "oidc.authorization-endpoint",
		"oidc.token-endpoint", "oidc.userinfo-endpoint", "oidc.jwks-uri", "oidc.client-id",
	}
	for _, name := range required {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("verify identity binding %s: %w", name, err)
		}
		if strings.TrimSpace(string(data)) == "" {
			return fmt.Errorf("verify identity binding %s: value is empty", name)
		}
	}
	return nil
}

func validateIdentityDiscovery(d IdentityDiscovery) error {
	for label, value := range map[string]string{
		"issuer": d.Issuer,
		"authorization endpoint": d.AuthorizationEndpoint,
		"token endpoint": d.TokenEndpoint,
		"userinfo endpoint": d.UserinfoEndpoint,
		"jwks uri": d.JWKSURI,
	} {
		value = strings.TrimSpace(value)
		if value == "" || !strings.HasPrefix(value, "https://") {
			return fmt.Errorf("identity discovery %s must be an HTTPS URL", label)
		}
	}
	if d.EndSessionEndpoint != "" && !strings.HasPrefix(strings.TrimSpace(d.EndSessionEndpoint), "https://") {
		return fmt.Errorf("identity discovery end session endpoint must be an HTTPS URL")
	}
	return nil
}

func capabilityBindingEntryName(name string) error {
	// Service Binding 1.1 well-known names are used where they fit. OIDC
	// extensions are namespace-qualified so no BaseHarbor-only alias becomes
	// part of the application contract.
	switch name {
	case "type", "provider", "uri":
		return nil
	case "client-secret":
		// The secret file name is intentionally capability-local and never
		// confused with the standard username/password entries.
		return nil
	default:
		if !strings.HasPrefix(name, "oidc.") {
			return fmt.Errorf("identity binding entry %q is not namespaced", name)
		}
		return nil
	}
}
