package identityprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type ManagedOperatorOIDC struct {
	Issuer       string
	ClientID     string
	CallbackPort int
}

func EnsureManagedOperatorOIDC(ctx context.Context, runtime KeycloakRuntime, issuer serviceaccess.Issuer, dataDir, namespace, target, environment string) (ManagedOperatorOIDC, error) {
	app := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        "operator-access",
		Environment: strings.TrimSpace(environment),
		Services:    application.Services{Identity: true},
	}
	placement, err := application.ResolveProviderPlacement(app, capability.ProviderKeycloak)
	if err != nil {
		return ManagedOperatorOIDC{}, err
	}
	if placement.Scope != capability.ScopeShared {
		return ManagedOperatorOIDC{}, fmt.Errorf("managed BaseHarbor operator identity requires shared Keycloak placement; got %s", placement.Scope)
	}
	files, err := EnsureKeycloakFilesAt(ctx, app, issuer, dataDir, namespace)
	if err != nil {
		return ManagedOperatorOIDC{}, err
	}
	canonicalBase, err := managedOperatorCanonicalBaseURL(runtime, namespace)
	if err != nil {
		return ManagedOperatorOIDC{}, err
	}
	if err := SetKeycloakCanonicalURL(files, canonicalBase); err != nil {
		return ManagedOperatorOIDC{}, err
	}
	files.CanonicalPublicURL = canonicalBase
	if err := runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return ManagedOperatorOIDC{}, fmt.Errorf("validate managed operator Keycloak: %w", err)
	}
	if err := runtime.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return ManagedOperatorOIDC{}, fmt.Errorf("start managed operator Keycloak: %w", err)
	}

	admin, err := operatorKeycloakAdmin(ctx, files)
	if err != nil {
		return ManagedOperatorOIDC{}, err
	}
	realm := operatorRealm(environment)
	ownership := map[string]string{
		"baseharbor.owner":       "baseharbor",
		"baseharbor.scope":       "operator",
		"baseharbor.target":      strings.TrimSpace(target),
		"baseharbor.environment": strings.TrimSpace(environment),
	}
	desiredRealm := keycloakRealm{
		Realm:                                  realm,
		Enabled:                                true,
		DisplayName:                            "BaseHarbor operators (" + environment + ")",
		SSLRequired:                            "external",
		BruteForceProtected:                    true,
		RegistrationAllowed:                    true,
		ResetPasswordAllowed:                   true,
		RememberMe:                             false,
		OTPPolicyType:                          "totp",
		WebAuthnPolicyRpEntityName:             "BaseHarbor",
		WebAuthnPolicySignatureAlgorithms:      []string{"ES256", "RS256"},
		WebAuthnPolicyPasswordlessRpEntityName: "BaseHarbor",
		WebAuthnPolicyPasswordlessSignatureAlgorithms: []string{"ES256", "RS256"},
		Attributes: ownership,
	}
	if err := admin.reconcileRealm(ctx, desiredRealm); err != nil {
		return ManagedOperatorOIDC{}, err
	}

	values, err := readProtectedEnv(files.Env)
	if err != nil {
		return ManagedOperatorOIDC{}, err
	}
	callbackKey := "BASEHARBOR_OPERATOR_" + strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(environment), "-", "_")) + "_CALLBACK_PORT"
	callbackPort := 0
	if raw := strings.TrimSpace(values[callbackKey]); raw != "" {
		callbackPort, err = parseIdentityPort(raw)
		if err != nil {
			return ManagedOperatorOIDC{}, fmt.Errorf("invalid managed operator callback port: %w", err)
		}
	} else {
		callbackPort, err = allocateIdentityPort(map[int]struct{}{files.PublicPort: {}, files.AdminPort: {}})
		if err != nil {
			return ManagedOperatorOIDC{}, err
		}
		values[callbackKey] = strconv.Itoa(callbackPort)
		if err := writeProtectedEnv(files.Env, values); err != nil {
			return ManagedOperatorOIDC{}, err
		}
	}
	clientID := operatorClientID(environment)
	callback := "http://127.0.0.1:" + strconv.Itoa(callbackPort) + "/callback"
	client := keycloakClient{
		ClientID:                  clientID,
		Name:                      "BaseHarbor CLI " + environment,
		Enabled:                   true,
		Protocol:                  "openid-connect",
		PublicClient:              true,
		StandardFlowEnabled:       true,
		DirectAccessGrantsEnabled: false,
		ServiceAccountsEnabled:    false,
		RedirectURIs:              []string{callback},
		WebOrigins:                []string{"http://127.0.0.1:" + strconv.Itoa(callbackPort)},
		Attributes: map[string]string{
			"baseharbor.scope":           "operator",
			"baseharbor.target":          strings.TrimSpace(target),
			"baseharbor.environment":     strings.TrimSpace(environment),
			"pkce.code.challenge.method": "S256",
		},
	}
	if _, err := admin.reconcileClient(ctx, realm, client); err != nil {
		return ManagedOperatorOIDC{}, err
	}
	publicClient, err := keycloakPublicHTTPClient(files)
	if err != nil {
		return ManagedOperatorOIDC{}, err
	}
	endpointIssuer := files.PublicURL + "/realms/" + url.PathEscape(realm)
	issuerURL := files.CanonicalPublicURL + "/realms/" + url.PathEscape(realm)
	if _, err := FetchDiscoveryAt(ctx, publicClient, endpointIssuer, issuerURL); err != nil {
		return ManagedOperatorOIDC{}, err
	}
	return ManagedOperatorOIDC{Issuer: issuerURL, ClientID: clientID, CallbackPort: callbackPort}, nil
}

func managedOperatorCanonicalBaseURL(runtime KeycloakRuntime, namespace string) (string, error) {
	host, err := devaccess.SharedHost(namespace, "identity")
	if err != nil {
		return "", err
	}
	return devgateway.URLForRuntime(namespace, host, runtime), nil
}

func EnsureManagedDevelopmentAccess(ctx context.Context, runtime KeycloakRuntime, issuer serviceaccess.Issuer, dataDir, namespace, target, username, password string) (ManagedOperatorOIDC, error) {
	managed, err := EnsureManagedOperatorOIDC(ctx, runtime, issuer, dataDir, namespace, target, "dev")
	if err != nil {
		return ManagedOperatorOIDC{}, err
	}
	app := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        "operator-access",
		Environment: "dev",
		Services:    application.Services{Identity: true},
	}
	files, err := ExistingKeycloakFilesAt(app, dataDir, namespace)
	if err != nil {
		return ManagedOperatorOIDC{}, err
	}
	admin, err := operatorKeycloakAdmin(ctx, files)
	if err != nil {
		return ManagedOperatorOIDC{}, err
	}
	if err := admin.reconcileUser(ctx, operatorRealm("dev"), username, password, true); err != nil {
		return ManagedOperatorOIDC{}, err
	}
	return managed, nil
}

func FinalizeManagedOperatorOIDC(ctx context.Context, runtime KeycloakRuntime, issuer serviceaccess.Issuer, dataDir, namespace, target, environment string) error {
	app := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        "operator-access",
		Environment: strings.TrimSpace(environment),
		Services:    application.Services{Identity: true},
	}
	files, err := EnsureKeycloakFilesAt(ctx, app, issuer, dataDir, namespace)
	if err != nil {
		return err
	}
	admin, err := operatorKeycloakAdmin(ctx, files)
	if err != nil {
		return err
	}
	realm := operatorRealm(environment)
	status, body, err := admin.do(ctx, "GET", "/admin/realms/"+url.PathEscape(realm), nil)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("inspect managed operator realm: HTTP %d: %s", status, body)
	}
	var current keycloakRealm
	if err := decodeJSON(body, &current); err != nil {
		return err
	}
	expected := map[string]string{
		"baseharbor.owner":       "baseharbor",
		"baseharbor.scope":       "operator",
		"baseharbor.target":      strings.TrimSpace(target),
		"baseharbor.environment": strings.TrimSpace(environment),
	}
	if !keycloakRealmOwnedBy(current, expected) {
		return fmt.Errorf("managed operator realm ownership verification failed")
	}
	if !current.RegistrationAllowed {
		return nil
	}
	current.RegistrationAllowed = false
	status, body, err = admin.do(ctx, "PUT", "/admin/realms/"+url.PathEscape(realm), current)
	if err != nil {
		return err
	}
	if status != 204 {
		return fmt.Errorf("close managed operator self-registration: HTTP %d: %s", status, body)
	}
	return nil
}

func operatorKeycloakAdmin(ctx context.Context, files KeycloakFiles) (*keycloakAdmin, error) {
	client, err := serviceaccess.NewHTTPClient(files.AdminAccess.Material, false)
	if err != nil {
		return nil, err
	}
	if err := waitIdentityEndpoint(ctx, client, files.AdminURL+"/realms/master/.well-known/openid-configuration"); err != nil {
		return nil, err
	}
	values, err := readProtectedEnv(files.Env)
	if err != nil {
		return nil, err
	}
	admin := &keycloakAdmin{
		endpoint: files.AdminURL,
		client:   client,
		user:     values["BASEHARBOR_KEYCLOAK_ADMIN_USER"],
		password: values["BASEHARBOR_KEYCLOAK_ADMIN_PASSWORD"],
	}
	if err := admin.login(ctx); err != nil {
		return nil, err
	}
	return admin, nil
}

func operatorRealm(environment string) string {
	return "bh-operators-" + strings.ToLower(strings.TrimSpace(environment))
}

func operatorClientID(environment string) string {
	return "baseharbor-cli-" + strings.ToLower(strings.TrimSpace(environment))
}

func decodeJSON(value string, target any) error {
	return json.Unmarshal([]byte(value), target)
}
