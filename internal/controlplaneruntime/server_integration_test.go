package controlplaneruntime

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/database"
)

func TestRunStartsTLSHealthReadinessAndProtectedAPI(t *testing.T) {
	baseDSN := os.Getenv("BASEHARBOR_TEST_DATABASE_URL")
	if baseDSN == "" {
		t.Skip("BASEHARBOR_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	dsn, cleanupDatabase := createRuntimeTestDatabase(t, ctx, baseDSN)
	defer cleanupDatabase()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	pool.Close()

	var issuer string
	oidcServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q,"jwks_uri":%q}`, issuer, issuer+"/authorize", issuer+"/token", issuer+"/keys")
		case "/keys":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"keys":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer oidcServer.Close()
	issuer = oidcServer.URL

	certFile, keyFile := writeTestCertificate(t)
	addr := freeTCPAddress(t)

	runCtx, cancel := context.WithCancel(oidc.ClientContext(ctx, oidcServer.Client()))
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(runCtx, Config{
			ListenAddr:    addr,
			DatabaseURL:   dsn,
			OIDCIssuer:    issuer,
			OIDCAudiences: []string{"baseharbor-test"},
			TLSCertFile:   certFile,
			TLSKeyFile:    keyFile,
		}, application.Store{Root: t.TempDir()})
	}()

	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tlsConfigForTest},
		Timeout:   2 * time.Second,
	}
	baseURL := "https://" + addr
	waitForHTTPS(t, client, baseURL+"/healthz")

	resp := mustGET(t, client, baseURL+"/healthz", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp = mustGET(t, client, baseURL+"/readyz", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("readyz status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp = mustGET(t, client, baseURL+"/api/v1/apps/demo/secrets", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("protected API status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run shutdown error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("control-plane server did not shut down")
	}
}

var tlsConfigForTest = tls.Config{InsecureSkipVerify: true} // test-only self-signed listener

func createRuntimeTestDatabase(t *testing.T, ctx context.Context, baseDSN string) (string, func()) {
	t.Helper()
	u, err := url.Parse(baseDSN)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("baseharbor_runtime_%d", time.Now().UnixNano())
	maintenance := *u
	maintenance.Path = "/postgres"
	admin, err := pgxpool.New(ctx, maintenance.String())
	if err != nil {
		t.Fatal(err)
	}
	quoted := `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	target := *u
	target.Path = "/" + name
	return target.String(), func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)")
		admin.Close()
	}
}

func writeTestCertificate(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}

func freeTCPAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func waitForHTTPS(t *testing.T, client *http.Client, endpoint string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(endpoint)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("HTTPS endpoint did not become ready: %s", endpoint)
}

func mustGET(t *testing.T, client *http.Client, endpoint, bearer string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}
