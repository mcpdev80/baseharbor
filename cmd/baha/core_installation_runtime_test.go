package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/auth"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/development/pythonadapter"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/containersecurity"
)

func TestCoreOnlyBootstrapRuntimeAcceptance(t *testing.T) {
	if os.Getenv("BASEHARBOR_CORE_BOOTSTRAP_ACCEPTANCE") != "1" {
		t.Skip("set BASEHARBOR_CORE_BOOTSTRAP_ACCEPTANCE=1 on an isolated Docker/Podman runtime")
	}
	command := "docker"
	format := "{{json .SecurityOptions}}"
	if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
		command, format = "podman", "{{.Host.Security.Rootless}}"
	}
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer probeCancel()
	rootless, err := exec.CommandContext(probeCtx, command, "info", "--format", format).Output()
	if err != nil || (command == "docker" && !strings.Contains(string(rootless), "rootless")) || (command == "podman" && strings.TrimSpace(string(rootless)) != "true") {
		t.Fatal("Core bootstrap acceptance requires a verified rootless runtime host")
	}
	roles := []coreinstallation.MachineRole{coreinstallation.Development, coreinstallation.Deployment}
	if os.Getenv("BASEHARBOR_FINAL_BUG_ACCEPTANCE") == "1" {
		roles = roles[:1]
	}
	for _, role := range roles {
		if !t.Run(string(role), func(t *testing.T) { runCoreOnlyBootstrapRuntime(t, role) }) {
			return
		}
	}
}

func runCoreOnlyBootstrapRuntime(t *testing.T, role coreinstallation.MachineRole) {
	runCoreOnlyBootstrapRuntimeWithHA(t, role, false)
}

func runCoreOnlyBootstrapRuntimeWithHA(t *testing.T, role coreinstallation.MachineRole, ha bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	if os.Getenv("BASEHARBOR_FINAL_BUG_ACCEPTANCE") == "1" {
		t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	}
	target := configureTestTarget(t)
	if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
		cfg, err := deployment.LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		definition := cfg.Targets[target.Name]
		definition.Runtime.Provider = "podman"
		cfg.Targets[target.Name] = definition
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		target, err = cfg.ResolveTarget("", "")
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(t.TempDir()) // Deliberately no repository or application contract.
	runtime, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	containers, err := runtime.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, container := range containers {
		if container.Project == targetRuntimeProjectName(target) {
			t.Fatal("isolated Core acceptance host required; existing project refused")
		}
	}
	var out bytes.Buffer
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cleanupCancel()
		if role == coreinstallation.Deployment {
			var err error
			cleanupCtx, err = authenticatedCoreBootstrapFixtureCleanup(cleanupCtx, target)
			if err != nil {
				t.Errorf("Core fixture operator authentication failed; owners retained: %v", err)
				return
			}
		}
		if err := approvedNativeFixtureCleanup(t, cleanupCtx, &bytes.Buffer{}); err != nil {
			t.Errorf("Core cleanup failed: %v", err)
		}
	}()
	sampler := startCoreMemorySampler(ctx, runtime, target.RuntimeProvider, targetRuntimeProjectName(target), bhruntime.SharedProjectName(target.Name), bhruntime.SharedProjectName(target.Name+"-core"))
	defer sampler.stop()
	opts := runtimeUpOptions{HA: ha, Yes: true, ControlPlaneOnly: true, MachineRole: role, RecoveryFile: filepath.Join(t.TempDir(), "recovery.json")}
	var preservedRecovery string
	if role == coreinstallation.Development {
		preservedRecovery, err = defaultTargetRecoveryFile(target.Name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(preservedRecovery), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(preservedRecovery, []byte("previous installation recovery material\n"), 0600); err != nil {
			t.Fatal(err)
		}
		opts.RecoveryFile = ""
	}
	first, err := installCore(ctx, strings.NewReader(""), &out, opts)
	if err != nil {
		logCoreBootstrapFailure(t, runtime, target.Name, target.RuntimeProvider)
		t.Fatalf("Core-only bootstrap failed: %v", err)
	}
	if !first.Ready || first.IdentityIssuer == "" || first.Spec.MachineRole != role {
		t.Fatalf("Core not ready: %+v", first)
	}
	report, err := inspectControlPlane(ctx)
	if err != nil || !report.Ready || report.Installation == nil || report.Installation.ID != first.ID {
		t.Fatalf("independent inspection failed: %+v %v", report, err)
	}
	verifyIndependentManagementProviderUnion(t, ctx, target, first)
	files, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	services := []string{"postgres", "openbao", "openbao-member-1"}
	if !ha {
		services = append(services, "postgres-member-1")
	}
	for _, service := range services {
		if err := containersecurity.VerifyComposeService(ctx, files.Project, service, containersecurity.Requirements{ReadOnlyRootfs: true, DropAllCaps: true, NoNewPrivs: true}); err != nil {
			inventory, inventoryErr := runtime.ListRuntimeContainers(ctx)
			for _, container := range inventory {
				t.Logf("Runtime resource: project=%s service=%s running=%t", container.Project, container.Service, container.Running)
			}
			if inventoryErr != nil {
				t.Logf("Runtime inventory unavailable: %v", inventoryErr)
			}
			t.Fatal(err)
		}
	}
	if ha {
		// Native Spilo bootstraps as container root with a writable filesystem.
		// Its database process must still run as the unprivileged postgres user
		// inside the verified rootless host namespace. Do not impose the distinct
		// Single PostgreSQL image's container filesystem contract on Spilo.
		const processUID = `ps -eo comm=,uid= | awk '$1 == "postgres" { found=1; if ($2 == 0) bad=1 } END { exit (!found || bad) }'`
		if _, err := runtime.ExecProject(ctx, files.Project, files.Compose, files.Env, "postgres-member-1", "sh", "-ec", processUID); err != nil {
			t.Fatalf("HA PostgreSQL database process must be non-root: %v", err)
		}
	}
	second, err := installCore(ctx, strings.NewReader(""), &out, opts)
	if err != nil || !second.Ready || second.ID != first.ID {
		t.Fatalf("retry changed installation: %+v %v", second, err)
	}
	if err := requireApplicationCore(machineNoninteractiveContext(ctx), strings.NewReader(""), &out); err != nil {
		t.Fatalf("existing Core not reused: %v", err)
	}
	if _, err := os.Stat("baseharbor.yaml"); !os.IsNotExist(err) {
		t.Fatal("Core bootstrap created an Application")
	}
	dataDir, err := targetDataRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	// Secret-bearing provider environment files must not leak into human output.
	for _, env := range []string{files.Env, filepath.Join(dataDir, "providers", "keycloak", "shared", "core", "runtime.env")} {
		data, err := os.ReadFile(env)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			key, value, ok := strings.Cut(line, "=")
			if ok && len(value) >= 12 && (strings.Contains(key, "PASSWORD") || strings.Contains(key, "SECRET")) && strings.Contains(out.String(), value) {
				t.Fatal("bootstrap output contains a protected credential")
			}
		}
	}
	public, err := json.Marshal(map[string]any{"installation": first.ID, "role": role, "runtime": target.RuntimeProvider, "capabilities": first.Capabilities, "placement": first.Placement, "ready": true})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Core-only verified result: %s", public)
	memory, err := sampler.evidence(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Core resource evidence: %s", memory)
	if preservedRecovery != "" {
		data, err := os.ReadFile(preservedRecovery)
		if err != nil || string(data) != "previous installation recovery material\n" {
			t.Fatal("fresh bootstrap changed previous recovery material")
		}
		current, source, err := resolveTargetRecoveryFile(ctx, "")
		if err != nil || current == preservedRecovery || source != "persisted target" {
			t.Fatalf("fresh recovery reference not persisted: %s %s %v", current, source, err)
		}
	}
	if os.Getenv("BASEHARBOR_CORE_IDENTITY_SQL_CA_ACCEPTANCE") == "1" {
		verifyInstalledCoreIdentitySQLCARotation(t, ctx, target, first, opts.RecoveryFile)
	}
	if role == coreinstallation.Development && os.Getenv("BASEHARBOR_DUAL_DOCKER_ACCEPTANCE") == "1" {
		verifyDualDockerCLIAndMCP(t, ctx, target)
	}
	if role == coreinstallation.Development && os.Getenv("BASEHARBOR_FINAL_BUG_ACCEPTANCE") == "1" {
		runFinalBugRuntimeRegression(t, ctx, target, opts)
	}
	if role == coreinstallation.Development && os.Getenv("BASEHARBOR_BUG_HUNT_LIFECYCLE_ACCEPTANCE") == "1" {
		runManagedProviderOnlyReadinessRegression(t, ctx)
		runManagedReadinessAndBackupRegression(t, ctx)
		runGeneratedSecretDeliveryRegression(t, ctx)
	}
}

// The deployment fixture must authenticate its real installed operator before
// entering the unchanged, plan-bound installation destroy authority. An OAuth
// access token alone is not a principal: verify the issued ID token through the
// real issuer/JWKS with the normal signature, audience and expiry checks.
func authenticatedCoreBootstrapFixtureCleanup(ctx context.Context, target deployment.ResolvedTarget) (context.Context, error) {
	dataDir, err := targetDataRoot(target)
	if err != nil {
		return ctx, err
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return ctx, err
	}
	state, err := coreinstallation.Load(root)
	if err != nil || state.Spec.Target != target.Name || state.Spec.Runtime != target.RuntimeProvider {
		return ctx, errors.New("Core fixture installation owner is unavailable or differs")
	}
	if err := identityprovider.VerifyCoreOperatorTokenFlow(ctx, dataDir, target.Name, state.ID, state.IdentityIssuer); err != nil {
		return ctx, err
	}
	files, err := identityprovider.ExistingCoreRuntimeFiles(dataDir, target.Name)
	if err != nil {
		return ctx, err
	}
	values := map[string]string{}
	for _, path := range []string{files.Env, filepath.Join(files.Dir, "active-admin.env")} {
		data, err := runtimeMutationOwnerBytes(path, path != files.Env)
		if err != nil {
			return ctx, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			key, value, ok := strings.Cut(line, "=")
			if ok {
				values[strings.TrimSpace(key)] = strings.TrimSpace(value)
			}
		}
		zeroBytes(data)
	}
	defer clear(values)
	username, password := values["BASEHARBOR_KEYCLOAK_ADMIN_USER"], values["BASEHARBOR_KEYCLOAK_ADMIN_PASSWORD"]
	if username == "" || password == "" || !strings.HasSuffix(state.IdentityIssuer, "/realms/baseharbor") {
		return ctx, errors.New("Core fixture protected operator or issuer is incomplete")
	}
	// Core's baseharbor realm deliberately has its native frontendUrl. Master
	// uses KC_HOSTNAME, whose distinct canonical origin is protected runtime input.
	issuer := strings.TrimRight(values["BASEHARBOR_KEYCLOAK_CANONICAL_URL"], "/") + "/realms/master"
	if err := (auth.Config{Issuer: issuer, Audiences: []string{"admin-cli"}}).Validate(); err != nil {
		return ctx, errors.New("Core fixture protected master issuer is incomplete")
	}
	issuerURL, err := url.Parse(issuer)
	if err != nil {
		return ctx, err
	}
	adminURL, err := url.Parse(files.AdminURL)
	if err != nil {
		return ctx, err
	}
	client, err := serviceaccess.NewHTTPClient(files.AdminAccess.Material, false)
	if err != nil {
		return ctx, err
	}
	transport := client.Transport.(*http.Transport).Clone()
	transport.Proxy = nil
	dial := transport.DialContext
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != adminURL.Host && address != issuerURL.Host && address != net.JoinHostPort(issuerURL.Hostname(), "443") {
			return nil, errors.New("Core fixture authentication refused another HTTP authority")
		}
		return dial(ctx, network, adminURL.Host)
	}
	client.Transport = transport
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	defer transport.CloseIdleConnections()
	form := url.Values{"grant_type": {"password"}, "client_id": {"admin-cli"}, "username": {username}, "password": {password}, "scope": {"openid"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, files.AdminURL+"/realms/master/protocol/openid-connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return ctx, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return ctx, errors.New("Core fixture operator OAuth endpoint is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ctx, fmt.Errorf("Core fixture operator OAuth rejected: HTTP %d", response.StatusCode)
	}
	var token struct {
		IDToken string `json:"id_token"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token) != nil || token.IDToken == "" {
		return ctx, errors.New("Core fixture operator OAuth did not supply an ID token")
	}
	return verifyCoreBootstrapFixtureIDToken(ctx, client, issuer, token.IDToken)
}

func verifyCoreBootstrapFixtureIDToken(ctx context.Context, client *http.Client, issuer, token string) (context.Context, error) {
	verifiedCtx := oidc.ClientContext(ctx, client)
	verifier, err := auth.NewOIDCVerifier(verifiedCtx, auth.Config{Issuer: issuer, Audiences: []string{"admin-cli"}})
	if err != nil {
		return ctx, errors.New("Core fixture operator OIDC discovery failed")
	}
	principal, err := verifier.Verify(verifiedCtx, token)
	if err != nil {
		return ctx, errors.New("Core fixture operator ID token failed verification")
	}
	return operatorauth.WithVerifiedPrincipal(ctx, principal), nil
}

func TestCoreBootstrapFixtureCleanupRequiresVerifiedOIDC(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var issuer string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "fixture", Algorithm: "RS256", Use: "sig"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	issuer = server.URL
	sign := func(audience, tokenIssuer string, expiry int64) string {
		t.Helper()
		claims, err := json.Marshal(map[string]any{"iss": tokenIssuer, "sub": "verified-fixture-operator", "aud": audience, "exp": expiry, "iat": time.Now().Unix()})
		if err != nil {
			t.Fatal(err)
		}
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "fixture"))
		if err != nil {
			t.Fatal(err)
		}
		object, err := signer.Sign(claims)
		if err != nil {
			t.Fatal(err)
		}
		token, err := object.CompactSerialize()
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	valid := sign("admin-cli", issuer, time.Now().Add(time.Minute).Unix())
	ctx, err := verifyCoreBootstrapFixtureIDToken(t.Context(), server.Client(), issuer, valid)
	principal, authenticated := operatorauth.PrincipalFromContext(ctx)
	if err != nil || !authenticated || principal.Subject != "verified-fixture-operator" || principal.Issuer != issuer {
		t.Fatal("actual signed issuer token did not yield the verified principal")
	}
	for _, token := range []string{"", "access-token-is-not-an-ID-token", sign("another-client", issuer, time.Now().Add(time.Minute).Unix()), sign("admin-cli", "https://another-issuer.invalid", time.Now().Add(time.Minute).Unix()), sign("admin-cli", issuer, time.Now().Add(-time.Minute).Unix())} {
		ctx, err := verifyCoreBootstrapFixtureIDToken(t.Context(), server.Client(), issuer, token)
		if _, authenticated := operatorauth.PrincipalFromContext(ctx); err == nil || authenticated {
			t.Fatal("invalid issuer, audience, expiry or token manufactured an authenticated operator")
		}
	}
}

// Observe only this fixture's resources before deferred cleanup removes them.
// Emit lifecycle state and known error categories, never raw provider logs or
// credential-bearing environments.
func logCoreBootstrapFailure(t *testing.T, runtime bhruntime.RuntimeProvider, target, engine string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	containers, err := runtime.ListRuntimeContainers(ctx)
	if err != nil {
		t.Log("Core failure inventory unavailable")
		return
	}
	backend := bhruntime.NewCLIBackend(engine)
	projects := map[string]bool{bhruntime.SharedProjectName(target): true, bhruntime.SharedProjectName(target + "-core"): true}
	for _, container := range containers {
		if !projects[container.Project] {
			continue
		}
		t.Logf("Core failure resource: project=%s service=%s running=%t health=%s", container.Project, container.Service, container.Running, container.Health)
		if engine == "podman" {
			state, err := exec.CommandContext(ctx, "systemctl", "--user", "show", container.Project+"-"+container.Service+".service", "--property=ActiveState,SubState,Result,ExecMainStatus,ExecMainCode,ExecMainExitTimestampMonotonic", "--no-pager").Output()
			if err == nil {
				t.Logf("Core failure unit: service=%s %s", container.Service, strings.TrimSpace(string(state)))
			}
		}
		logs, err := backend.DirectOutput(ctx, "logs", "--tail", "60", container.ID)
		if err != nil {
			continue
		}
		for _, category := range []string{"permission denied", "Permission denied", "certificate verify failed", "could not translate host name", "password authentication failed", "Connection refused", "No such file or directory", "did not become ready", "OutOfMemoryError", "SQLState: 28P01", "SQLState: 08006"} {
			if strings.Contains(logs, category) {
				t.Logf("Core failure category: service=%s category=%s", container.Service, category)
			}
		}
	}
}

func runManagedProviderOnlyReadinessRegression(t *testing.T, ctx context.Context) {
	t.Helper()
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeShared))
	manifest := application.New("provider-only-readiness", "dev", true, false, false)
	if err := writeInitializedAuthoredManifest(t, application.RepositoryManifestName, manifest, 0600); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		// Exercise default provider-only retirement with no runtime directory,
		// then dispose of the isolated recovery owner before reusing its source.
		if err := destroySelectiveApplicationWithConsent(cleanup); err != nil {
			t.Errorf("provider-only retirement: %v", err)
			return
		}
		resetCoreBootstrapFixtureApplication(t, cleanup)
	}()
	var output runtimeAcceptanceOutput
	if err := runWithIO(coreBootstrapFixtureConsent(t, ctx), []string{"app", "apply"}, &output, &output); err != nil {
		t.Fatalf("provider-only apply: %v\n%s", err, output.String())
	}
	resolved, err := resolveApplication(ctx, application.DefaultStore(), nil, "readiness regression")
	if err != nil {
		t.Fatal(err)
	}
	files := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
	runtime, err := detectRuntimeForApplication(ctx, resolved)
	if err != nil {
		t.Fatal(err)
	}
	containers, err := runtime.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, container := range containers {
		if container.Project == files.Project {
			t.Fatal("provider-only fixture unexpectedly has application-project containers")
		}
	}
	status, err := collectApplicationStatus(ctx, application.DefaultStore(), nil)
	if err != nil || !status.Ready {
		t.Fatalf("provider-only status: %+v %v", status, err)
	}
	output.Reset()
	if err := runWithIO(ctx, []string{"app", "show", "--output", "json"}, &output, &output); err != nil {
		t.Fatal(err)
	}
	var overview applicationOverview
	if err := json.Unmarshal([]byte(output.String()), &overview); err != nil || !overview.Ready {
		t.Fatalf("provider-only app show disagrees with READY: %+v %v", overview, err)
	}
	output.Reset()
	if err := runWithIO(ctx, []string{"app", "doctor"}, &output, &output); err != nil {
		t.Fatalf("provider-only doctor: %v\n%s", err, output.String())
	}
	t.Log("Provider-only shared SQL: app show/status/doctor agree without any application-project container")
}

func runManagedReadinessAndBackupRegression(t *testing.T, ctx context.Context) {
	t.Helper()
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeShared))
	manifest := application.New("backup-broker-regression", "dev", true, false, false)
	manifest = application.WithWorkloadComponents(manifest, "api")
	manifest = application.WithRuntimePermission(manifest, "object-storage.s3/v1", []string{"api"}, "runtime.create", "runtime.get", "runtime.delete")
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	if manifest.Services.Secrets || !application.RequiresRuntimeBroker(manifest) {
		t.Fatal("fixture must require a broker without application secrets")
	}
	if err := writeInitializedAuthoredManifest(t, application.RepositoryManifestName, manifest, 0600); err != nil {
		t.Fatal(err)
	}
	compose := `services:
  api:
    image: docker.io/library/python:3.13-alpine
    command: ["python", "-m", "http.server", "8080"]
    healthcheck:
      test: ["CMD", "python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8080/')"]
      interval: 1s
      timeout: 3s
      retries: 30
`
	if err := os.WriteFile("compose.yaml", []byte(compose), 0600); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		resetCoreBootstrapFixtureApplication(t, cleanup)
	}()
	var output runtimeAcceptanceOutput
	if err := runWithIO(coreBootstrapFixtureConsent(t, ctx), []string{"app", "apply"}, &output, &output); err != nil {
		runtime, runtimeErr := detectRuntimeForTarget(ctx, mustEffectiveTestTarget(t, ctx))
		if runtimeErr == nil {
			containers, _ := runtime.ListRuntimeContainers(ctx)
			backend := bhruntime.NewCLIBackend(os.Getenv("BASEHARBOR_TEST_RUNTIME"))
			for _, container := range containers {
				if !strings.HasPrefix(container.Service, "keycloak") {
					continue
				}
				logs, logErr := backend.DirectOutput(ctx, "logs", "--tail", "80", container.ID)
				if logErr != nil {
					continue
				}
				for _, category := range []string{"SQLState: 28P01", "SQLState: 08006", "password authentication failed", "SSLHandshakeException", "UnknownHostException", "Connection refused", "OutOfMemoryError", "failed to validate certificate", "permission denied"} {
					if strings.Contains(logs, category) {
						t.Logf("Identity diagnostic: service=%s category=%s", container.Service, category)
					}
				}
			}
		}
		var operational *machine.Error
		if errors.As(err, &operational) && operational.Cause != nil {
			resolved, resolveErr := resolveApplication(ctx, application.DefaultStore(), nil, "failure diagnostics")
			if resolveErr == nil {
				files, filesErr := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
				if filesErr == nil {
					environment, envErr := repositoryWorkloadEnvironment(ctx, resolved, files)
					if envErr == nil {
						t.Logf("Workload start diagnostic: %s", sanitizeWorkloadDiagnostic(operational.Cause.Error(), environment))
					}
				}
			}
		}
		t.Fatalf("managed fixture apply: %v\n%s", err, output.String())
	}
	check := func() {
		status, err := collectApplicationStatus(ctx, application.DefaultStore(), nil)
		if err != nil || !status.Ready {
			t.Fatalf("canonical readiness: %+v %v", status, err)
		}
		resolved, err := resolveApplication(ctx, application.DefaultStore(), nil, "bug hunt")
		if err != nil {
			t.Fatal(err)
		}
		overview, err := inspectApplicationOverview(ctx, resolved)
		if err != nil || !overview.Ready {
			t.Fatalf("overview disagrees with READY: %+v %v", overview, err)
		}
		var doctor runtimeAcceptanceOutput
		if err := runWithIO(ctx, []string{"app", "doctor"}, &doctor, &doctor); err != nil {
			t.Fatalf("doctor after backup: %v\n%s", err, doctor.String())
		}
	}
	check()
	password := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(password, []byte("native-regression-backup-password"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first", "second"} {
		archive := filepath.Join(t.TempDir(), name+".bhbackup")
		output.Reset()
		if err := runWithIO(ctx, []string{"app", "backup", "--password-file", password, "--output", archive}, &output, &output); err != nil {
			t.Fatalf("backup: %v\n%s", err, output.String())
		}
		info, err := os.Stat(archive)
		if err != nil || info.Size() == 0 {
			t.Fatal("backup archive missing")
		}
		check()
	}
	t.Log("Shared provider app show/status/doctor agree; two encrypted backups restore the runtime-permission broker without application secrets")
}

func mustEffectiveTestTarget(t *testing.T, ctx context.Context) deployment.ResolvedTarget {
	t.Helper()
	target, err := effectiveTarget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func runGeneratedSecretDeliveryRegression(t *testing.T, ctx context.Context) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "generated")
	registry, err := development.NewRegistry(pythonadapter.Adapter{})
	if err != nil {
		t.Fatal(err)
	}
	created, err := development.CreateApplication(root, development.NewApplicationRequest{Name: "generated-secret-regression", Adapter: pythonadapter.AdapterID, Capabilities: []capability.Kind{capability.SQL, capability.Secrets, capability.ExposureHTTP}, Secrets: []string{"API_TOKEN"}}, registry)
	if err != nil {
		t.Fatal(err)
	}
	if !created.Validation.Satisfied {
		t.Fatalf("generated contract: %+v", created.Validation)
	}
	t.Chdir(root)
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		resetCoreBootstrapFixtureApplication(t, cleanup)
	}()
	var output runtimeAcceptanceOutput
	err = runWithIO(generatedCoreBootstrapFixtureConsent(t, ctx), []string{"up", "--yes"}, &output, &output)
	var missing *machine.Error
	if !errors.As(err, &missing) || missing.Code != machine.ErrorRequiredSecretMissing {
		t.Fatalf("non-TTY first up must fail closed for missing secret: %v\n%s", err, output.String())
	}
	payload := []byte("native-generated-secret-value")
	secretPath := filepath.Join(t.TempDir(), "secret-input")
	if err := os.WriteFile(secretPath, payload, 0600); err != nil {
		t.Fatal(err)
	}
	if err := runWithIO(ctx, []string{"app", "secret", "set", "API_TOKEN", "--file", secretPath}, &output, &output); err != nil {
		t.Fatalf("supply required managed secret: %v\n%s", err, output.String())
	}
	if err := runWithIO(coreBootstrapFixtureConsent(t, ctx), []string{"up", "--yes"}, &output, &output); err != nil {
		t.Fatalf("resume generated application: %v\n%s", err, output.String())
	}
	status, err := collectApplicationStatus(ctx, application.DefaultStore(), nil)
	if err != nil || !status.Ready {
		t.Fatalf("generated app not ready: %+v %v", status, err)
	}
	resolved, err := resolveApplication(ctx, application.DefaultStore(), nil, "secret delivery regression")
	if err != nil {
		t.Fatal(err)
	}
	files, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	workload, found, err := application.MaterializeWorkload(root, resolved.Manifest, files)
	if err != nil || !found {
		t.Fatalf("generated workload missing: %v", err)
	}
	runtime, err := detectRuntimeForApplication(ctx, resolved, bhruntime.CapabilityServiceExec)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	program := "import hashlib,os; assert hashlib.sha256(os.environ['API_TOKEN'].encode()).hexdigest() == '" + hex.EncodeToString(sum[:]) + "'; print('managed delivery verified')"
	result, err := runtime.ExecProjectFiles(ctx, workload.Project, root, "app", []string{workload.Compose, workload.Override}, "python", "-c", program)
	if err != nil || strings.TrimSpace(result) != "managed delivery verified" {
		t.Fatalf("generated code did not receive normal managed-secret binding: %v", err)
	}
	if strings.Contains(output.String(), string(payload)) {
		t.Fatal("secret value appeared in CLI output")
	}
	t.Log("Generated native Python app receives its named managed secret after actionable non-TTY failure and resumes READY without source edits")
}

// stdout and stderr can write concurrently during runtime convergence. The
// acceptance recorder has the same concurrency guarantee as process streams.
type runtimeAcceptanceOutput struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *runtimeAcceptanceOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}
func (b *runtimeAcceptanceOutput) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}
func (b *runtimeAcceptanceOutput) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buffer.Reset()
}

// Native fixtures review and acknowledge their actual current provider plan;
// ordinary --yes is never promoted to either independent acknowledgement.
func coreBootstrapFixtureConsent(t *testing.T, ctx context.Context) context.Context {
	t.Helper()
	resolved, err := resolveApplication(ctx, application.Store{}, nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildResolvedApplicationPlan(ctx, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if impact := plan.ProviderPlan.Impact; impact != nil {
		return withProviderChangeConsent(ctx, &application.ProviderChangeConsent{PlanID: impact.PlanID, First: true, Second: true})
	}
	return ctx
}

func TestGeneratedBootstrapConsentRetainsCurrentIntent(t *testing.T) {
	configureTestTarget(t)
	root := filepath.Join(t.TempDir(), "generated")
	registry, err := development.NewRegistry(pythonadapter.Adapter{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = development.CreateApplication(root, development.NewApplicationRequest{Name: "generated-secret-regression", Adapter: pythonadapter.AdapterID, Capabilities: []capability.Kind{capability.SQL, capability.Secrets, capability.ExposureHTTP}, Secrets: []string{"API_TOKEN"}}, registry)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	ctx := machineNoninteractiveContext(t.Context())
	beforeInitialization := coreBootstrapFixtureConsent(t, ctx)
	consent := generatedCoreBootstrapFixtureConsent(t, ctx)
	resolved, err := resolveApplication(ctx, application.Store{}, nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	var conflict *machine.Error
	if err := authorizeApplicationProviderChanges(beforeInitialization, resolved, nil, &bytes.Buffer{}); !errors.As(err, &conflict) || conflict.Code != machine.ErrorConflict {
		t.Fatalf("deployment input initialization did not invalidate earlier consent: %v", err)
	}
	for attempt := 0; attempt < 4; attempt++ {
		resolved, err := resolveApplication(ctx, application.Store{}, nil, "apply")
		if err != nil {
			t.Fatal(err)
		}
		if err := revalidateApplicationProviderIntent(ctx, resolved); err != nil {
			t.Fatalf("unchanged generated intent: %v", err)
		}
		if err := authorizeApplicationProviderChanges(consent, resolved, nil, &bytes.Buffer{}); err != nil {
			t.Fatalf("unchanged generated consent: %v", err)
		}
	}
}

// Ordinary up resolves deployment inputs before entering application apply.
// A fixture must acknowledge the resulting owner/input state, not the proposal
// that existed before this normal preparation. It never retries stale consent.
func generatedCoreBootstrapFixtureConsent(t *testing.T, ctx context.Context) context.Context {
	t.Helper()
	if err := ensureRepositoryDeploymentInputsForUp(ctx, strings.NewReader(""), &bytes.Buffer{}, runtimeUpOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	return coreBootstrapFixtureConsent(t, ctx)
}

func resetCoreBootstrapFixtureApplication(t *testing.T, ctx context.Context) {
	t.Helper()
	current, err := resolveApplication(ctx, application.Store{}, nil, "destroy")
	if err != nil {
		t.Errorf("fixture reset owner: %v", err)
		return
	}
	plan, err := buildApplicationDestructionPlan(ctx, current, true)
	if err != nil {
		t.Errorf("fixture reset plan: %v", err)
		return
	}
	ack := &application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true, DeleteData: true}
	var out runtimeAcceptanceOutput
	if err := runWithIO(withProviderChangeConsent(ctx, ack), []string{"app", "destroy", "--yes", "--full-reset"}, &out, &out); err != nil {
		t.Errorf("fixture reset failed; owner/source retained: %v\n%s", err, out.String())
	}
}
