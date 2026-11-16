package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/exposure"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
)

// This separate bounded production fixture uses real BYOC TLS, actual
// published owner endpoints and real user OAuth. It never guesses a callback
// from a development gateway or relaxes Keycloak redirect matching.
func TestProductionIdentityRuntimeRealizedCallbackAndReapply(t *testing.T) {
	if os.Getenv("BASEHARBOR_IDENTITY_LIFECYCLE_ACCEPTANCE") != "1" {
		t.Skip("requires isolated exact-SHA Docker/rootless Podman Identity acceptance")
	}
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 15*time.Minute)
	defer cancel()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
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
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	assertCorelessHost(t, ctx, provider, target)
	t.Chdir(t.TempDir())
	if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	intent := "version: 1\napp:\n  name: production-identity\n  environment: prod\nworkload:\n  components:\n    - api\nservices:\n  identity: true\nidentity:\n  callback_paths:\n    - /oauth/callback\nexposure:\n  http:\n    - name: oauth\n      service: api\n      port: 8080\n      protocol: https\n      visibility: public\n"
	if err := os.WriteFile("baseharbor.yaml", []byte(intent), 0600); err != nil {
		t.Fatal(err)
	}
	certDir := productionIdentityCertificate(t, "oauth.example.test")
	var out bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "init", "--hostname", "oauth.example.test", "--tls", "existing", "--cert-dir", certDir, "--yes"}, &out, &out); err != nil {
		t.Fatalf("production BYOC initialization failed: %v", err)
	}
	initial, err := resolveApplication(ctx, application.Store{}, nil, "plan")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cleanupCancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 3*time.Minute)
		defer cleanupCancel()
		if err := destroySelectiveApplicationWithConsent(cleanup); err != nil {
			t.Errorf("production owner retirement cleanup: %v", err)
		}
		if _, err := exposure.RetireRuntime(cleanup, provider, initial.Manifest, application.RuntimeFilesFor(initial.Store, initial.Manifest)); err != nil && !os.IsNotExist(err) {
			t.Errorf("production exposure cleanup: %v", err)
		}
		placement, err := application.ResolveProviderPlacement(initial.Manifest, capability.ProviderKeycloak)
		if err != nil {
			t.Errorf("production cleanup placement unavailable: %v", err)
			return
		}
		if _, err := identityprovider.RetireKeycloakRuntime(cleanup, provider, initial.Manifest, placement, initial.TargetStateRoot, target.Name); err != nil && !os.IsNotExist(err) {
			t.Errorf("production identity cleanup: %v", err)
		}
		if containers, err := provider.ListRuntimeContainers(cleanup); err != nil || len(containers) != 0 {
			t.Errorf("production cleanup requires zero actual containers: count=%d inspection=%v", len(containers), err)
		}
	})
	var session *identityOAuthRuntimeSession
	var secret, ownerEnv [32]byte
	for _, operation := range []string{"apply", "apply", "up"} {
		current, err := resolveApplication(ctx, application.Store{}, nil, operation)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := buildResolvedApplicationPlan(ctx, current)
		if err != nil {
			t.Fatal(err)
		}
		applyCtx := ctx
		if plan.ProviderPlan.Impact != nil {
			applyCtx = withProviderChangeConsent(ctx, &application.ProviderChangeConsent{PlanID: plan.ProviderPlan.Impact.PlanID, First: true, Second: true})
		}
		out.Reset()
		if err := runWithIO(applyCtx, []string{"app", operation, "--skip-memory-preflight"}, &out, &out); err != nil {
			t.Fatalf("production %s failed: %v", operation, err)
		}
		current, err = resolveApplication(ctx, application.Store{}, nil, "status")
		if err != nil || current.Manifest.ApplicationID != initial.Manifest.ApplicationID {
			t.Fatal("production application identity changed")
		}
		status, _, err := collectResolvedApplicationStatus(ctx, current)
		if err != nil || !status.Ready {
			t.Fatal("production runtime did not verify actual readiness", err)
		}
		files, err := identityprovider.ExistingKeycloakFilesAt(current.Manifest, current.TargetStateRoot, target.Name)
		if err != nil {
			t.Fatal(err)
		}
		callback := productionIdentityOAuthCallback(t, current)
		verifyProductionIdentityCallbackTLS(t, ctx, current, certDir)
		data, err := os.ReadFile(files.Env)
		if err != nil {
			t.Fatal(err)
		}
		clientSecret, err := os.ReadFile(filepath.Join(application.RuntimeFilesFor(current.Store, current.Manifest).Bindings, application.IdentityBindingName, "client-secret"))
		if err != nil {
			t.Fatal(err)
		}
		if session == nil {
			ownerEnv, secret = sha256.Sum256(data), sha256.Sum256(clientSecret)
			session = startIdentityOAuthRuntimeSession(t, ctx, current, files)
		} else {
			if ownerEnv != sha256.Sum256(data) || secret != sha256.Sum256(clientSecret) || callback != session.callback {
				t.Fatal("production reapply replaced retained credentials or actual callback")
			}
			session.refresh(t, ctx)
			session.login(t, ctx, false)
		}
		clear(clientSecret)
		t.Logf("runtime=%s production %s: actual BYOC callback TLS, end-user OAuth, canonical owner and retained client/session verified", provider.Kind(), operation)
	}
}

func productionIdentityOAuthCallback(t *testing.T, resolved resolvedApplication) string {
	t.Helper()
	inventory, err := exposure.ExistingRuntimeInventory(resolved.Manifest, application.RuntimeFilesFor(resolved.Store, resolved.Manifest))
	if err != nil || inventory.State.Host == "" || len(inventory.State.Routes) != 1 {
		t.Fatal("production callback requires protected actual exposure owner")
	}
	route := inventory.State.Routes[0]
	if route.Name != "oauth" || route.Service != "api" || route.TargetPort != 8080 || route.Protocol != "https" || route.ProviderProtocol != "https" || route.Visibility != "public" || route.PublishedPort < 1 || route.PublishedPort > 65535 {
		t.Fatal("production callback differs from actual declared HTTPS route")
	}
	return fmt.Sprintf("https://%s/oauth/callback", net.JoinHostPort(inventory.State.Host, fmt.Sprint(route.PublishedPort)))
}

func productionIdentityCertificate(t *testing.T, host string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal("BYOC entropy unavailable")
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	cert, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal("BYOC certificate creation failed")
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal("BYOC key encoding failed")
	}
	dir := t.TempDir()
	for name, data := range map[string][]byte{"cert.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert}), "key.pem": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal("BYOC fixture material unavailable")
		}
	}
	return dir
}

func verifyProductionIdentityCallbackTLS(t *testing.T, ctx context.Context, resolved resolvedApplication, certDir string) {
	t.Helper()
	inventory, err := exposure.ExistingRuntimeInventory(resolved.Manifest, application.RuntimeFilesFor(resolved.Store, resolved.Manifest))
	if err != nil {
		t.Fatal("production callback owner unavailable")
	}
	ca, err := os.ReadFile(filepath.Join(certDir, "cert.pem"))
	roots := x509.NewCertPool()
	if err != nil || !roots.AppendCertsFromPEM(ca) {
		t.Fatal("BYOC trust unavailable")
	}
	port := inventory.State.Routes[0].PublishedPort
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, actualPort, err := net.SplitHostPort(address)
		if err != nil || host != inventory.State.Host || actualPort != fmt.Sprint(port) {
			return nil, &identityOAuthFixtureError{}
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", actualPort))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response := identityOAuthRequest(t, ctx, client, http.MethodGet, productionIdentityOAuthCallback(t, resolved), nil, "", "")
	defer response.Body.Close()
	// The Nginx fixture does not implement an OAuth handler; the real redirect
	// was already verified at Keycloak. This independent probe proves the
	// actual public HTTPS endpoint and hostname/CA verification, not handler UX.
	if response.TLS == nil || len(response.TLS.VerifiedChains) == 0 || response.StatusCode >= 500 {
		t.Fatal("actual published callback lacks verified TLS/workload transport")
	}
}
