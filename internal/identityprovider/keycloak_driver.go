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
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
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
	if strings.Join(d.origins, "\x00") != strings.Join(normalized, "\x00") {
		// Newly realized callback origins need actual client reconciliation,
		// even when the provider was already provisioned before exposure.
		d.provisioned = false
	}
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
	if err := admin.reconcileRealm(ctx, realm, ownership); err != nil {
		return err
	}
	if _, _, err := admin.ensureManagedSigningProvider(ctx, d.realm); err != nil {
		return fmt.Errorf("ensure BaseHarbor-managed Keycloak signing provider: %w", err)
	}
	if err := admin.ensureClientSecretRotationPolicy(ctx, d.realm); err != nil {
		return err
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
	loginCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var last error
	for {
		attemptCtx, attemptCancel := context.WithTimeout(loginCtx, 5*time.Second)
		err := admin.login(attemptCtx)
		attemptCancel()
		if err == nil {
			return admin, nil
		}
		last = err
		var loginErr *keycloakAdminLoginError
		retryable := transientKeycloakAdminLogin(err) || (errors.As(err, &loginErr) &&
			loginErr.Status == http.StatusBadRequest && strings.Contains(loginErr.Body, "invalid_grant"))
		if !retryable {
			return nil, err
		}
		select {
		case <-loginCtx.Done():
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("Keycloak admin authentication did not converge within 20s: %w", last)
		case <-ticker.C:
		}
	}
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

type keycloakProjectExecutor interface {
	ExecProject(ctx context.Context, project, composeFile, envFile, service string, args ...string) (string, error)
}

type keycloakProjectUserExecutor interface {
	ExecProjectAs(ctx context.Context, project, composeFile, envFile, service, user string, args ...string) (string, error)
}

func reloadKeycloakAccessGateway(ctx context.Context, runtime KeycloakRuntime, files KeycloakFiles) error {
	executor, ok := runtime.(keycloakProjectExecutor)
	if !ok {
		return errors.New("Keycloak PKI rotation requires runtime gateway reload support")
	}
	if _, err := executor.ExecProject(
		ctx,
		files.Project,
		files.Compose,
		files.Env,
		"keycloak-access",
		"/run/baseharbor/caddy",
		"reload",
		"--config",
		"/etc/caddy/Caddyfile",
		"--adapter",
		"caddyfile",
		"--force",
	); err != nil {
		return fmt.Errorf("reload Keycloak access gateway trust: %w", err)
	}
	return nil
}

// Spilo copies the PostgreSQL private key into its runtime directory at
// startup. Refresh that copy before signaling every member to reload TLS,
// while both CAs are trusted and before retiring the previous CA.
func reloadKeycloakDatabaseCertificates(ctx context.Context, runtime KeycloakRuntime, files KeycloakFiles) error {
	executor, ok := runtime.(keycloakProjectUserExecutor)
	if !ok {
		return errors.New("Keycloak PKI rotation requires runtime database reload as the postgres user")
	}
	reloadCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	values, err := readProtectedEnv(files.Env)
	if err != nil {
		return err
	}
	services := []string{"keycloak-db"}
	keyPath := keycloakDatabaseTLSKeyPaths{
		certificate: "/run/baseharbor/db-tls/server.pem",
		sourceKey:   "/run/baseharbor/db-tls/server-key.pem",
		runtimeKey:  "/tmp/server-key.pem",
	}
	if values["BASEHARBOR_KEYCLOAK_TOPOLOGY"] == "ha" {
		services = []string{"keycloak-db-member-1", "keycloak-db-member-2", "keycloak-db-member-3"}
		keyPath = keycloakDatabaseTLSKeyPaths{
			certificate: "/run/baseharbor/db-tls-source/server.pem",
			sourceKey:   "/run/baseharbor/db-tls-source/server-key.pem",
			runtimeKey:  "/run/baseharbor/etcd-runtime/server-key.pem",
		}
	} else if values["BASEHARBOR_KEYCLOAK_TOPOLOGY"] != "single" {
		return errors.New("Keycloak PostgreSQL TLS reload requires a known, owned topology")
	}
	reloadCommand := keycloakDatabaseTLSReloadCommand(keyPath)
	for _, service := range services {
		var lastOutput string
		var lastErr error
		for {
			attemptCtx, attemptCancel := context.WithTimeout(reloadCtx, 10*time.Second)
			output, err := executor.ExecProjectAs(attemptCtx, files.Project, files.Compose, files.Env, service, "postgres",
				"sh", "-ec", reloadCommand)
			attemptCancel()
			if err == nil {
				break
			}
			lastOutput = strings.TrimSpace(output)
			lastErr = err
			select {
			case <-reloadCtx.Done():
				return fmt.Errorf("reload Keycloak PostgreSQL certificates on %s: %w (last command output: %q)", service, errors.Join(reloadCtx.Err(), lastErr), lastOutput)
			case <-ticker.C:
			}
		}
	}
	return nil
}

type keycloakDatabaseTLSKeyPaths struct {
	certificate string
	sourceKey   string
	runtimeKey  string
}

func keycloakDatabaseTLSReloadCommand(paths keycloakDatabaseTLSKeyPaths) string {
	return fmt.Sprintf(`if [ ! -s "$PGDATA/postmaster.pid" ]; then echo 'postgres postmaster pid is not ready' >&2; exit 1; fi
pid=$(sed -n '1p' "$PGDATA/postmaster.pid")
case "$pid" in ''|*[!0-9]*) exit 1 ;; esac
[ -r "/proc/$pid/comm" ] || { echo 'postgres process is not ready' >&2; exit 1; }
comm=$(cat "/proc/$pid/comm")
case "$comm" in postgres|postmaster) ;; *) echo "unexpected postgres process: $comm" >&2; exit 1 ;; esac
source_cert=%q
source_key=%q
runtime_key=%q
[ -f "$source_cert" ] && [ -r "$source_cert" ] || { echo 'postgres server certificate is unavailable' >&2; exit 1; }
[ -f "$source_key" ] && [ -r "$source_key" ] || { echo 'postgres server key is unavailable' >&2; exit 1; }
[ ! -L "$runtime_key" ] || { echo 'postgres runtime key must not be a symlink' >&2; exit 1; }
umask 077
tmp_key=$(mktemp "${runtime_key}.next.XXXXXX")
cert_pub="${tmp_key}.cert.pub"
key_pub="${tmp_key}.key.pub"
probe_output="${tmp_key}.probe"
served_cert="${tmp_key}.served.pem"
trap 'rm -f "$tmp_key" "$cert_pub" "$key_pub" "$probe_output" "$served_cert"' EXIT HUP INT TERM
cp "$source_key" "$tmp_key"
chmod 0600 "$tmp_key"
openssl x509 -in "$source_cert" -pubkey -noout > "$cert_pub"
openssl pkey -in "$tmp_key" -pubout > "$key_pub"
cmp -s "$cert_pub" "$key_pub" || { echo 'postgres server certificate and key do not match' >&2; exit 1; }
mv -f "$tmp_key" "$runtime_key"
kill -HUP "$pid"
if ! timeout 5 openssl s_client -starttls postgres -connect 127.0.0.1:5432 -showcerts </dev/null > "$probe_output" 2>&1; then echo 'postgres TLS probe failed after certificate reload' >&2; cat "$probe_output" >&2; exit 1; fi
sed -n '/-----BEGIN CERTIFICATE-----/,/-----END CERTIFICATE-----/p' "$probe_output" > "$served_cert"
[ -s "$served_cert" ] || { echo 'postgres TLS probe returned no server certificate' >&2; exit 1; }
expected_fingerprint=$(openssl x509 -in "$source_cert" -noout -fingerprint -sha256)
served_fingerprint=$(openssl x509 -in "$served_cert" -noout -fingerprint -sha256)
[ "$served_fingerprint" = "$expected_fingerprint" ] || { echo "postgres still serves a stale certificate (expected $expected_fingerprint, got $served_fingerprint)" >&2; exit 1; }`, paths.certificate, paths.sourceKey, paths.runtimeKey)
}

type keycloakTrustReloadRuntime interface {
	UpProjectFilesSelectedForceRecreateNoBuild(context.Context, string, string, map[string]string, []string, ...string) error
	ServiceStatesProjectFilesEnv(context.Context, string, string, map[string]string, ...string) ([]bhruntime.ServiceState, error)
}

// Reload the JDBC trust manager from the mounted overlap bundle before the
// database starts presenting a leaf signed only by the replacement CA. A
// Compose up does not restart an unchanged Keycloak container when only a
// bind-mounted file changed, and the JDBC driver's trust manager is cached by
// the running JVM.
func reloadKeycloakDatabaseTrust(ctx context.Context, reloader keycloakTrustReloadRuntime, files KeycloakFiles) error {
	values, err := readProtectedEnv(files.Env)
	if err != nil {
		return err
	}
	var services []string
	switch values["BASEHARBOR_KEYCLOAK_TOPOLOGY"] {
	case "ha":
		services = []string{"keycloak-1", "keycloak-2", "keycloak-3"}
	case "single":
		services = []string{"keycloak-1"}
	default:
		return errors.New("Keycloak JDBC trust reload requires a known, owned topology")
	}
	for _, service := range services {
		if err := reloader.UpProjectFilesSelectedForceRecreateNoBuild(ctx, files.Project, files.Dir, values, []string{service}, files.Compose); err != nil {
			return fmt.Errorf("restart %s to reload the Keycloak database trust bundle: %w", service, err)
		}
		if err := waitKeycloakServiceReady(ctx, reloader, files, values, service); err != nil {
			return err
		}
	}
	return nil
}

func waitKeycloakServiceReady(ctx context.Context, runtime keycloakTrustReloadRuntime, files KeycloakFiles, environment map[string]string, service string) error {
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var lastState []bhruntime.ServiceState
	var lastErr error
	for {
		states, err := runtime.ServiceStatesProjectFilesEnv(ctx, files.Project, files.Dir, environment, files.Compose)
		if err == nil {
			lastState = states
			for _, state := range states {
				if state.Service == service && state.Ready() && strings.EqualFold(state.Health, "healthy") {
					return nil
				}
			}
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for %s after reloading Keycloak database trust: %w", service, ctx.Err())
		case <-deadline.C:
			return fmt.Errorf("wait for %s after reloading Keycloak database trust: timed out (last state: %+v, last error: %v)", service, lastState, lastErr)
		case <-ticker.C:
		}
	}
}

func waitKeycloakNativeCertificateReload(ctx context.Context) error {
	timer := time.NewTimer(2*keycloakCertificateReloadPeriod + 2*time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (d *KeycloakDriver) RotatePKI(ctx context.Context) error {
	r, ok := d.realization.(*localKeycloakRealization)
	if !ok || r == nil {
		return errors.New("Keycloak PKI rotation requires the managed local realization")
	}
	if r.issuer == nil {
		return errors.New("Keycloak PKI rotation requires a managed issuer")
	}

	reconcile := func(waitForNativeReload bool) (KeycloakFiles, error) {
		files, err := EnsureKeycloakFilesAt(ctx, r.app, r.issuer, r.dataDir, r.namespace)
		if err != nil {
			return KeycloakFiles{}, err
		}
		publicBase, err := localKeycloakPublicBaseURL(r.app, r.namespace, r.runtime, files)
		if err != nil {
			return KeycloakFiles{}, err
		}
		if err := prepareApplicationKeycloakCanonicalURL(r.runtime, r.namespace, files, publicBase); err != nil {
			return KeycloakFiles{}, err
		}
		if err := r.lifecycle.Validate(ctx, files); err != nil {
			return KeycloakFiles{}, err
		}
		if err := r.lifecycle.Apply(ctx, files); err != nil {
			return KeycloakFiles{}, err
		}
		if waitForNativeReload {
			trustRuntime, ok := r.runtime.(keycloakTrustReloadRuntime)
			if !ok {
				return KeycloakFiles{}, errors.New("Keycloak PKI rotation requires runtime trust-bundle reload support")
			}
			if err := reloadKeycloakDatabaseTrust(ctx, trustRuntime, files); err != nil {
				return KeycloakFiles{}, err
			}
		}
		if err := reloadKeycloakDatabaseCertificates(ctx, r.runtime, files); err != nil {
			return KeycloakFiles{}, err
		}
		// Keycloak polls its native HTTPS leaf files. During overlap creation the
		// running members can still present the previous leaf, so do not reload
		// the gateway onto the replacement trust set until every member has had
		// a full native reload window.
		if waitForNativeReload {
			if err := waitKeycloakNativeCertificateReload(ctx); err != nil {
				return KeycloakFiles{}, err
			}
		}
		if err := reloadKeycloakAccessGateway(ctx, r.runtime, files); err != nil {
			return KeycloakFiles{}, err
		}
		instance, err := r.instance(ctx, files, publicBase)
		if err != nil {
			return KeycloakFiles{}, err
		}
		r.files = files
		d.instance = instance
		if _, err := d.adminClient(ctx); err != nil {
			return KeycloakFiles{}, fmt.Errorf("verify Keycloak after PKI reconcile: %w", err)
		}
		return files, nil
	}

	files, err := reconcile(true)
	if err != nil {
		return fmt.Errorf("reconcile replacement Keycloak PKI with overlap: %w", err)
	}
	if _, err := reconcile(false); err != nil {
		return fmt.Errorf("verify Keycloak after native certificate reload: %w", err)
	}

	publicPolicy, err := serviceaccess.Resolve(r.app.Environment, "keycloak-public", serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	publicPolicy.ServerName = keycloakPublicHost
	frontendPolicy := publicPolicy
	frontendPolicy.AuthenticationRequired = false
	frontendPolicy.Authentication = serviceaccess.AuthenticationNative
	dbPolicy, err := serviceaccess.Resolve(r.app.Environment, "keycloak-db", serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	dbPolicy.ServerName = "keycloak-db"

	for _, item := range []struct {
		name   string
		policy serviceaccess.Policy
		dir    string
	}{
		{name: "native", policy: publicPolicy, dir: filepath.Join(files.Dir, "native-tls", "pki")},
		{name: "database", policy: dbPolicy, dir: filepath.Join(files.Dir, "db-ha", "pki")},
		{name: "frontend", policy: frontendPolicy, dir: filepath.Join(files.Dir, "service-access", "pki")},
	} {
		if err := serviceaccess.RetireTLSOverlap(ctx, r.issuer, item.policy, item.dir); err != nil {
			return fmt.Errorf("retire previous Keycloak %s CA: %w", item.name, err)
		}
	}

	if _, err := reconcile(false); err != nil {
		return fmt.Errorf("reconcile Keycloak after CA retirement: %w", err)
	}
	return nil
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
