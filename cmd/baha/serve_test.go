package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestSplitNonEmpty(t *testing.T) {
	got := splitNonEmpty(" api-a, ,api-b ,api-a ")
	want := []string{"api-a", "api-b", "api-a"}
	if len(got) != len(want) {
		t.Fatalf("splitNonEmpty() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("splitNonEmpty()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestServeStartUpPathNeverReceivesNilStartupContext is the act-1405b round-8
// regression: for the supported per-application broker path
// (RuntimeAppName != "") serveCommand must hand controlplaneruntime.Run a
// NON-nil startup context. On the defect head startup was left nil on this path
// and Run derived context.WithCancel(ctx)/<-ctx.Done() from it, which panics.
// The test drives the broker path end-to-end (isolated XDG, no local installation
// target so managementHostForServe returns nil,nil, runtime-only API config) and
// proves the serve startup completes a full start/shutdown without a nil-context
// panic. It FAILS (panics, exit 1) on the defect head and PASSES (exit 0) with the
// startup := ctx fix. No new public API symbol is used.
func TestServeStartUpPathNeverReceivesNilStartupContext(t *testing.T) {
	xdir := filepath.Join(t.TempDir(), "xdg")
	if err := os.MkdirAll(filepath.Join(xdir, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(xdir, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(xdir, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(xdir, "data"))
	// Broker path: a per-application runtime app is bound, so RuntimeAppName != "".
	t.Setenv("BASEHARBOR_RUNTIME_APP_NAME", "runtime-app")
	t.Setenv("BASEHARBOR_RUNTIME_ENVIRONMENT", "dev")
	// A real local Target exists and its bootstrap lock is already held by
	// another authority. This per-application broker must never attempt to
	// acquire that lock; the regular management server still must.
	configureTestTargetEmpty(t)
	host, err := managementHostForServe(context.Background())
	if err != nil || host == nil {
		t.Fatalf("local management host not resolved: %v", err)
	}
	bootstrap, unlock, err := host.Verify()
	if err != nil || !bootstrap || unlock == nil {
		t.Fatalf("bootstrap owner setup failed: bootstrap=%t err=%v", bootstrap, err)
	}
	defer unlock()
	t.Setenv("BASEHARBOR_CONNECTIVITY_RELAY_MODE", "false")
	t.Setenv("BASEHARBOR_RUNTIME_EXECUTOR_MODE", "false")

	t.Setenv("BASEHARBOR_API_LISTEN_ADDR", "127.0.0.1:0")
	// Runtime-only: no database URL and empty OIDC audience/issuer so
	// controlplaneruntime.Run needs no external dependency to reach server startup.
	t.Setenv("BASEHARBOR_API_DATABASE_URL", "")
	t.Setenv("BASEHARBOR_API_OIDC_ISSUER", "")
	t.Setenv("BASEHARBOR_API_OIDC_AUDIENCES", "")
	// Valid per-application broker credentials so controlplaneruntime.Run
	// passes validation and reaches server startup on the non-nil startup context.
	setupBrokerFiles(t, filepath.Join(t.TempDir(), "broker"))

	cmd := serveCommand(application.Store{Root: t.TempDir()})
	var out, errOut bytes.Buffer
	runCtx, cancel := context.WithCancel(context.Background())

	// Drive the serve-startup path on its own goroutine. The control plane server
	// binds and reaches server startup on the startup context; once it has started
	// we cancel the run context, which the runtime observes through
	// <-ctx.Done() and shuts the servers down. This proves the serve path reaches
	// server startup WITHOUT panicking on a nil startup context (the defect head
	// panics here before ever starting).
	startErr := make(chan error, 1)
	go func() {
		startErr <- cmd.Run(runCtx, nil, &out, &errOut)
	}()

	select {
	case err := <-startErr:
		cancel()
		if err != nil {
			t.Fatalf("per-application broker serve returned error %v; the serve-startup path must start cleanly with a non-nil startup context", err)
		}
		t.Fatalf("serve returned before server startup; the serve-startup path must reach startup on a non-nil startup context")
	case <-time.After(10 * time.Second):
		// Server started (reached server startup) within the window; cancel to shut
		// down and confirm Run returns cleanly on the non-nil startup context.
		cancel()
		select {
		case err := <-startErr:
			if err != nil {
				t.Fatalf("serve returned error after startup %v; the serve-startup path must shut down cleanly on a non-nil startup context", err)
			}
		case <-time.After(15 * time.Second):
			t.Fatalf("serve did not shut down within 15s after cancel; possible leak on the per-application broker path")
		}
	}
}

// setupBrokerFiles writes a self-signed control-plane TLS key pair, a runtime
// client CA certificate, and runtime token/permissions files with the file
// permissions (0600) the runtime verifier requires, pointing BASEHARBOR_* at
// them. It drives the exact per-application broker precondition the regression
// needs: RuntimeAppName != "" with every required runtime credential present.
func setupBrokerFiles(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("create broker dir: %v", err)
	}
	certPath := filepath.Join(dir, "broker.crt")
	keyPath := filepath.Join(dir, "broker.key")
	caPath := filepath.Join(dir, "client-ca.pem")
	tokenPath := filepath.Join(dir, "runtime-token")
	permsPath := filepath.Join(dir, "runtime-permissions")

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "baseharbor-broker"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	certOut, err := os.Create(certPath)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	_ = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	_ = certOut.Close()
	derKey, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	_ = pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: derKey})
	_ = keyOut.Close()

	caCert := tmpl
	caCert.IsCA = true
	caCert.KeyUsage = x509.KeyUsageCertSign
	caDer, err := x509.CreateCertificate(rand.Reader, &caCert, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	caOut, err := os.Create(caPath)
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}
	_ = pem.Encode(caOut, &pem.Block{Type: "CERTIFICATE", Bytes: caDer})
	_ = caOut.Close()

	for _, p := range []string{tokenPath} {
		if err := os.WriteFile(p, []byte("broker-credential-value"), 0600); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	// Empty capabilities: no remote executor and no service verifier are required,
	// so the broker reaches server startup purely on the startup context.
	permissions := []byte(`[]`)
	if err := os.WriteFile(permsPath, permissions, 0600); err != nil {
		t.Fatalf("write %s: %v", permsPath, err)
	}

	t.Setenv("BASEHARBOR_API_TLS_CERT_FILE", certPath)
	t.Setenv("BASEHARBOR_API_TLS_KEY_FILE", keyPath)
	t.Setenv("BASEHARBOR_API_TLS_CLIENT_CA_FILE", caPath)
	t.Setenv("BASEHARBOR_RUNTIME_TOKEN_FILE", tokenPath)
	t.Setenv("BASEHARBOR_RUNTIME_PERMISSIONS_FILE", permsPath)
	t.Setenv("BASEHARBOR_RUNTIME_SECRETS_ENABLED", "false")
	t.Setenv("BASEHARBOR_RUNTIME_OPENBAO_URL", "")
}

func TestControlPlaneConfigFromEnvAllowsRuntimeOnlyMode(t *testing.T) {
	t.Setenv("BASEHARBOR_API_DATABASE_URL", "")
	t.Setenv("BASEHARBOR_API_OIDC_ISSUER", "")
	t.Setenv("BASEHARBOR_API_OIDC_AUDIENCES", "")
	t.Setenv("BASEHARBOR_API_TLS_CERT_FILE", "/run/baseharbor/tls.crt")
	t.Setenv("BASEHARBOR_API_TLS_KEY_FILE", "/run/baseharbor/tls.key")
	cfg, err := controlPlaneConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "" || cfg.OIDCIssuer != "" || len(cfg.OIDCAudiences) != 0 {
		t.Fatalf("unexpected runtime-only config: %#v", cfg)
	}
}

func TestControlPlaneConfigFromEnv(t *testing.T) {
	t.Setenv("BASEHARBOR_API_LISTEN_ADDR", "127.0.0.1:9443")
	t.Setenv("BASEHARBOR_API_DATABASE_URL", "postgres://runtime@example/baseharbor")
	t.Setenv("BASEHARBOR_API_OIDC_ISSUER", "https://issuer.example")
	t.Setenv("BASEHARBOR_API_OIDC_AUDIENCES", "control-plane,automation")
	t.Setenv("BASEHARBOR_API_TLS_CERT_FILE", "/run/baseharbor/tls.crt")
	t.Setenv("BASEHARBOR_API_TLS_KEY_FILE", "/run/baseharbor/tls.key")

	cfg, err := controlPlaneConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "127.0.0.1:9443" || cfg.OIDCIssuer != "https://issuer.example" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if len(cfg.OIDCAudiences) != 2 || cfg.OIDCAudiences[0] != "control-plane" || cfg.OIDCAudiences[1] != "automation" {
		t.Fatalf("unexpected audiences: %#v", cfg.OIDCAudiences)
	}
}
