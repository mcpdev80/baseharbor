package identityprovider

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type KeycloakDriver struct {
	realization KeycloakRealization
	app         application.Manifest
	appFiles    application.RuntimeFiles

	instance     KeycloakInstance
	origins      []string
	realm        string
	clientID     string
	clientSecret string
	discovery    application.IdentityDiscovery
	provisioned  bool
}

func NewKeycloakDriver(runtime KeycloakRuntime, app application.Manifest, appFiles application.RuntimeFiles, issuer serviceaccess.Issuer, dataDir, namespace string) *KeycloakDriver {
	return NewKeycloakDriverWithRealization(
		newLocalKeycloakRealization(runtime, app, issuer, dataDir, namespace),
		app,
		appFiles,
	)
}

func NewKeycloakDriverWithRealization(realization KeycloakRealization, app application.Manifest, appFiles application.RuntimeFiles) *KeycloakDriver {
	return &KeycloakDriver{
		realization: realization,
		app:         app,
		appFiles:    appFiles,
		realm:       keycloakRealmName(app),
		clientID:    keycloakClientID(app),
	}
}

func (d *KeycloakDriver) Descriptor() capability.Provider { return capability.Keycloak }

func (d *KeycloakDriver) IntegrationDescriptor() capability.IntegrationDescriptor {
	return capability.KeycloakIntegration
}

func (d *KeycloakDriver) SetApplicationOrigins(origins []string) error {
	var normalized []string
	seen := map[string]struct{}{}
	for _, raw := range origins {
		raw = strings.TrimRight(strings.TrimSpace(raw), "/")
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("identity application origin %q is invalid", raw)
		}
		if !isDevelopmentIdentityEnvironment(d.app.Environment) && u.Scheme != "https" {
			return fmt.Errorf("managed identity requires HTTPS application origins outside development")
		}
		if _, ok := seen[raw]; ok {
			continue
		}
		seen[raw] = struct{}{}
		normalized = append(normalized, raw)
	}
	normalized = sortedUnique(normalized)
	d.origins = normalized
	return nil
}

func (d *KeycloakDriver) Preflight(_ context.Context, resource capability.Resource, binding capability.Binding) error {
	if resource.Kind != capability.Identity || resource.Provider != capability.ProviderKeycloak {
		return fmt.Errorf("Keycloak cannot satisfy capability %q via provider %q", resource.Kind, resource.Provider)
	}
	if binding.Identity == nil {
		return errors.New("Keycloak identity binding metadata is required")
	}
	if err := capability.RequireIntegrationContract(d.IntegrationDescriptor()); err != nil {
		return err
	}
	if len(binding.Identity.CallbackPaths) > 0 || len(binding.Identity.LogoutPaths) > 0 {
		public := 0
		for _, exposure := range d.app.Exposures {
			if strings.EqualFold(strings.TrimSpace(exposure.Visibility), "internal") {
				continue
			}
			public++
			if !isDevelopmentIdentityEnvironment(d.app.Environment) && exposure.Protocol != "https" {
				return fmt.Errorf("managed identity callback/logout requires HTTPS public exposure outside development")
			}
		}
		if public == 0 {
			return errors.New("identity callback/logout paths require at least one public application exposure")
		}
	}
	for _, method := range binding.Identity.Methods {
		switch method {
		case "totp", "webauthn", "passkey":
		default:
			return fmt.Errorf("Keycloak reference provider does not support authentication method %q", method)
		}
	}
	return nil
}

type legacyServiceCleaner interface {
	RemoveProjectServices(context.Context, string, ...string) error
}

type keycloakRealizationDiagnostics interface {
	Diagnostics(context.Context) string
}

func (d *KeycloakDriver) Provision(ctx context.Context, resource capability.Resource, binding capability.Binding) error {
	if d.provisioned {
		return nil
	}
	if d.realization == nil {
		return errors.New("Keycloak provider realization is required")
	}
	instance, err := d.realization.Apply(ctx)
	if err != nil {
		detail := ""
		if diagnostics, ok := d.realization.(keycloakRealizationDiagnostics); ok {
			diagnosticCtx, diagnosticCancel := context.WithTimeout(context.Background(), 5*time.Second)
			detail = strings.TrimSpace(diagnostics.Diagnostics(diagnosticCtx))
			diagnosticCancel()
		}
		if detail != "" {
			return fmt.Errorf("realize Keycloak provider: %w\n%s", err, detail)
		}
		return fmt.Errorf("realize Keycloak provider: %w", err)
	}
	d.instance = instance
	publicBase := strings.TrimRight(strings.TrimSpace(instance.PublicBaseURL), "/")
	if publicBase == "" {
		return errors.New("Keycloak realization returned an empty public base URL")
	}

	admin, err := d.adminClient(ctx)
	if err != nil {
		return err
	}
	ownership := keycloakOwnership(d.app)
	realmAttributes := make(map[string]string, len(ownership)+1)
	for key, value := range ownership {
		realmAttributes[key] = value
	}
	if isDevelopmentIdentityEnvironment(d.app.Environment) {
		realmAttributes["frontendUrl"] = publicBase
	}
	realm := keycloakRealm{
		Realm: d.realm, Enabled: true,
		DisplayName: "BaseHarbor " + d.app.Name + " (" + d.app.Environment + ")",
		SSLRequired: "external", BruteForceProtected: true,
		RegistrationAllowed: false, ResetPasswordAllowed: true, RememberMe: true,
		OTPPolicyType:                                 "totp",
		WebAuthnPolicyRpEntityName:                    "BaseHarbor",
		WebAuthnPolicySignatureAlgorithms:             []string{"ES256", "RS256"},
		WebAuthnPolicyPasswordlessRpEntityName:        "BaseHarbor",
		WebAuthnPolicyPasswordlessSignatureAlgorithms: []string{"ES256", "RS256"},
		Attributes: realmAttributes,
	}
	if err := admin.reconcileRealm(ctx, realm); err != nil {
		return err
	}
	if _, _, err := admin.ensureManagedSigningProvider(ctx, d.realm); err != nil {
		return fmt.Errorf("ensure BaseHarbor-managed Keycloak signing provider: %w", err)
	}

	secret, err := d.ensureClientSecret()
	if err != nil {
		return err
	}
	d.clientSecret = secret
	redirects := identityURIs(d.origins, binding.Identity.CallbackPaths)
	logouts := identityURIs(d.origins, binding.Identity.LogoutPaths)
	attributes := map[string]string{
		"baseharbor.application": d.app.Name,
		"baseharbor.environment": d.app.Environment,
	}
	if len(logouts) > 0 {
		attributes["post.logout.redirect.uris"] = strings.Join(logouts, "##")
	}
	client := keycloakClient{
		ClientID: d.clientID,
		Name:     "BaseHarbor " + d.app.Name + " " + d.app.Environment,
		Enabled:  true, Protocol: "openid-connect",
		PublicClient: false, StandardFlowEnabled: true,
		DirectAccessGrantsEnabled: false, ServiceAccountsEnabled: false,
		Secret: secret, RedirectURIs: redirects,
		WebOrigins: identityOrigins(d.origins),
		Attributes: attributes,
	}
	clientUUID, err := admin.reconcileClient(ctx, d.realm, client)
	if err != nil {
		return err
	}
	if err := admin.reconcileClientScopes(ctx, d.realm, clientUUID, binding.Identity.Scopes, binding.Identity.Claims); err != nil {
		return err
	}
	if err := admin.reconcileRequiredActions(ctx, d.realm, binding.Identity.MFA, binding.Identity.Methods, binding.Identity.Passwordless); err != nil {
		return err
	}
	d.provisioned = true
	return nil
}

func (d *KeycloakDriver) Bind(ctx context.Context, _ capability.Resource, _ capability.Binding) error {
	if !d.provisioned {
		return errors.New("Keycloak identity was not provisioned")
	}
	client := d.instance.PublicHTTPClient
	if client == nil {
		return errors.New("Keycloak realization returned no public HTTP client")
	}
	endpointIssuer := strings.TrimRight(d.instance.EndpointBaseURL, "/") + "/realms/" + url.PathEscape(d.realm)
	publicIssuer := strings.TrimRight(d.instance.PublicBaseURL, "/") + "/realms/" + url.PathEscape(d.realm)
	workloadIssuer := strings.TrimRight(d.instance.WorkloadBaseURL, "/") + "/realms/" + url.PathEscape(d.realm)
	discovery, err := FetchDiscoveryAt(ctx, client, endpointIssuer, publicIssuer)
	if err != nil {
		return err
	}
	workloadDiscovery := rebaseIdentityDiscovery(discovery, publicIssuer, workloadIssuer)
	d.discovery = discovery
	return application.MaterializeIdentityBindingWithWorkloadDiscoveryMaterial(
		d.app, d.appFiles, string(capability.ProviderKeycloak),
		discovery, workloadDiscovery, d.clientID, d.clientSecret, d.instance.TrustBundle,
	)
}

func (d *KeycloakDriver) VerifyExisting(ctx context.Context, binding capability.Binding, origins []string) error {
	if err := d.SetApplicationOrigins(origins); err != nil {
		return err
	}
	if d.realization == nil {
		return errors.New("Keycloak provider realization is required")
	}
	instance, err := d.realization.Existing(ctx)
	if err != nil {
		return err
	}
	d.instance = instance
	return d.Verify(ctx, capability.Resource{}, binding)
}

func (d *KeycloakDriver) Verify(ctx context.Context, _ capability.Resource, binding capability.Binding) error {
	if binding.Identity == nil {
		return errors.New("identity binding is required")
	}
	client := d.instance.PublicHTTPClient
	if client == nil {
		return errors.New("Keycloak realization returned no public HTTP client")
	}
	endpointIssuer := strings.TrimRight(d.instance.EndpointBaseURL, "/") + "/realms/" + url.PathEscape(d.realm)
	publicIssuer := strings.TrimRight(d.instance.PublicBaseURL, "/") + "/realms/" + url.PathEscape(d.realm)
	discovery, err := FetchDiscoveryAt(ctx, client, endpointIssuer, publicIssuer)
	if err != nil {
		return err
	}
	if discovery.Issuer != d.discovery.Issuer && d.discovery.Issuer != "" {
		return errors.New("Keycloak issuer drift detected")
	}
	if err := application.VerifyIdentityBinding(d.app, d.appFiles); err != nil {
		return err
	}
	admin, err := d.adminClient(ctx)
	if err != nil {
		return err
	}
	return admin.verifyManagedIdentity(ctx, d.realm, keycloakOwnership(d.app), d.clientID, identityURIs(d.origins, binding.Identity.CallbackPaths), identityURIs(d.origins, binding.Identity.LogoutPaths), binding.Identity.MFA, binding.Identity.Methods, binding.Identity.Passwordless)
}

func (d *KeycloakDriver) DestroyApplication(ctx context.Context) error {
	if d.realization == nil {
		return errors.New("Keycloak provider realization is required")
	}
	if d.instance.StateDir == "" {
		instance, err := d.realization.Existing(ctx)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		d.instance = instance
	}
	admin, err := d.adminClient(ctx)
	if err != nil {
		return err
	}
	if err := admin.deleteRealm(ctx, d.realm, keycloakOwnership(d.app)); err != nil {
		return err
	}
	placement, err := application.ResolveProviderPlacement(d.app, capability.ProviderKeycloak)
	if err != nil {
		return err
	}
	if placement.Scope == capability.ScopeApplication {
		return d.realization.Destroy(ctx)
	}
	return os.RemoveAll(filepath.Join(d.instance.StateDir, "scopes", d.realm))
}

func (d *KeycloakDriver) adminClient(ctx context.Context) (*keycloakAdmin, error) {
	if d.instance.AdminHTTPClient == nil {
		return nil, errors.New("Keycloak realization returned no admin HTTP client")
	}
	endpoint := strings.TrimRight(d.instance.EndpointBaseURL, "/")
	if err := waitIdentityEndpoint(ctx, d.instance.AdminHTTPClient, endpoint+"/realms/master/.well-known/openid-configuration"); err != nil {
		if diagnostics, ok := d.realization.(keycloakRealizationDiagnostics); ok {
			if detail := strings.TrimSpace(diagnostics.Diagnostics(ctx)); detail != "" {
				return nil, fmt.Errorf("wait for Keycloak admin endpoint: %w; runtime diagnostics:\n%s", err, detail)
			}
		}
		return nil, fmt.Errorf("wait for Keycloak admin endpoint: %w", err)
	}
	admin := &keycloakAdmin{
		endpoint: endpoint,
		client:   d.instance.AdminHTTPClient,
		user:     d.instance.AdminUsername,
		password: d.instance.AdminPassword,
	}
	if err := admin.login(ctx); err != nil {
		return nil, err
	}
	return admin, nil
}

func (d *KeycloakDriver) ensureClientSecret() (string, error) {
	if strings.TrimSpace(d.instance.StateDir) == "" {
		return "", errors.New("Keycloak realization returned no state directory")
	}
	dir := filepath.Join(d.instance.StateDir, "scopes", d.realm)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "client-secret")
	if data, err := os.ReadFile(path); err == nil {
		value := strings.TrimSpace(string(data))
		if value == "" || strings.ContainsAny(value, "\r\n") {
			return "", errors.New("stored Keycloak client secret is invalid")
		}
		return value, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	value, err := randomIdentitySecret(32)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(value+"\n"), 0o600); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", err
	}
	return value, nil
}

func rebaseIdentityDiscovery(discovery application.IdentityDiscovery, fromIssuer, toIssuer string) application.IdentityDiscovery {
	fromIssuer = strings.TrimRight(strings.TrimSpace(fromIssuer), "/")
	toIssuer = strings.TrimRight(strings.TrimSpace(toIssuer), "/")
	rebase := func(value string) string {
		value = strings.TrimSpace(value)
		if value == fromIssuer {
			return toIssuer
		}
		if strings.HasPrefix(value, fromIssuer+"/") {
			return toIssuer + strings.TrimPrefix(value, fromIssuer)
		}
		return value
	}
	return application.IdentityDiscovery{
		Issuer:                rebase(discovery.Issuer),
		AuthorizationEndpoint: rebase(discovery.AuthorizationEndpoint),
		TokenEndpoint:         rebase(discovery.TokenEndpoint),
		UserinfoEndpoint:      rebase(discovery.UserinfoEndpoint),
		JWKSURI:               rebase(discovery.JWKSURI),
		EndSessionEndpoint:    rebase(discovery.EndSessionEndpoint),
	}
}

func keycloakRealmName(app application.Manifest) string {
	return "bh-" + app.Name + "-" + app.Environment
}

func keycloakClientID(app application.Manifest) string {
	return "baseharbor-" + app.Name + "-" + app.Environment
}

func keycloakOwnership(app application.Manifest) map[string]string {
	return map[string]string{
		"baseharbor.owner":       "baseharbor",
		"baseharbor.application": app.Name,
		"baseharbor.environment": app.Environment,
	}
}

func identityURIs(origins, paths []string) []string {
	var result []string
	for _, origin := range origins {
		for _, path := range paths {
			result = append(result, strings.TrimRight(origin, "/")+path)
		}
	}
	return sortedUnique(result)
}

func identityOrigins(origins []string) []string {
	return sortedUnique(origins)
}

func isDevelopmentIdentityEnvironment(environment string) bool {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "dev", "development":
		return true
	default:
		return false
	}
}

func keycloakPublicHTTPClient(files KeycloakFiles) (*http.Client, error) {
	pem, err := os.ReadFile(files.PublicAccess.Material.CA)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("Keycloak public CA contains no certificates")
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool, ServerName: keycloakPublicHost},
		TLSHandshakeTimeout: 5 * time.Second,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", strconv.Itoa(files.PublicPort)))
		},
	}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second}, nil
}

const identityEndpointReadyTimeout = 4 * time.Minute

func waitIdentityEndpoint(ctx context.Context, client *http.Client, endpoint string) error {
	waitCtx, cancel := context.WithTimeout(ctx, identityEndpointReadyTimeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var last error
	for {
		req, err := http.NewRequestWithContext(waitCtx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				return nil
			}
			last = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			last = err
		}
		select {
		case <-waitCtx.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if last != nil {
				return fmt.Errorf("identity endpoint readiness timeout after %s: %w", identityEndpointReadyTimeout, last)
			}
			return fmt.Errorf("identity endpoint readiness timeout after %s", identityEndpointReadyTimeout)
		case <-ticker.C:
		}
	}
}
