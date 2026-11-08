package operatorauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
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
)

// buildManagedCALeaf mints a self-signed managed test CA (written to caPath) and
// a leaf certificate for 127.0.0.1 signed by that CA. It returns the PEM cert
// and key pair for the httptest TLS server. The leaf is verified only through
// the additive managed-CA root pool resolved via the --ca gate, never via an
// injected server.Client() — exactly the production discovery trust path.
func buildManagedCALeaf(t *testing.T, caPath string) ([]byte, []byte) {
	t.Helper()
	_, caCert, caKey := writeTestCA(t, caPath)
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		DNSNames:              []string{"127.0.0.1"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		BasicConstraintsValid: true,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("generate leaf certificate: %v", err)
	}
	leafKeyPEM, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		t.Fatalf("marshal leaf key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: leafKeyPEM})
}

// TestLoginThroughManagedCAProvesCombinedProductionPath drives the real
// operator Login from provider discovery over JWKS to token exchange and PKCE
// completion entirely through an in-process OIDC server whose leaf is signed
// only by the managed test CA. No http.Client is injected into the context;
// the additive managed-CA root pool resolved through the --ca gate is the sole
// way to verify the server. This combines the CA-discovery (trust_test.go) and
// headless PKCE (login_headless_test.go) fixtures into the one production path
// that neither covers on its own.
func TestLoginThroughManagedCAProvesCombinedProductionPath(t *testing.T) {
	idKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	caPath := filepath.Join(dir, "managed-ca.pem")
	leafCertPEM, leafKeyPEM := buildManagedCALeaf(t, caPath)
	leafCert, err := tls.X509KeyPair(leafCertPEM, leafKeyPEM)
	if err != nil {
		t.Fatalf("build leaf tls pair: %v", err)
	}

	var issuer string
	verifierCh := make(chan string, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeOperatorAuthJSON(t, w, map[string]any{
				"issuer":                 issuer,
				"authorization_endpoint": issuer + "/authorize",
				"token_endpoint":         issuer + "/token",
				"jwks_uri":               issuer + "/keys",
			})
		case "/keys":
			writeOperatorAuthJSON(t, w, map[string]any{
				"keys": []map[string]any{{
					"kty": "RSA",
					"kid": "operator-test",
					"use": "sig",
					"alg": "RS256",
					"n":   base64.RawURLEncoding.EncodeToString(idKey.PublicKey.N.Bytes()),
					"e":   "AQAB",
				}},
			})
		case "/token":
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if r.Form.Get("grant_type") != "authorization_code" {
				http.Error(w, "unexpected grant", http.StatusBadRequest)
				return
			}
			verifier := r.Form.Get("code_verifier")
			if verifier == "" {
				http.Error(w, "missing PKCE verifier", http.StatusBadRequest)
				return
			}
			verifierCh <- verifier
			now := time.Now()
			idToken := signOperatorAuthIDToken(t, idKey, map[string]any{
				"iss": issuer,
				"sub": "ca-operator",
				"aud": "baseharbor-cli",
				"exp": now.Add(10 * time.Minute).Unix(),
				"iat": now.Add(-time.Minute).Unix(),
			})
			writeOperatorAuthJSON(t, w, map[string]any{
				"access_token": "opaque-access-token",
				"token_type":   "Bearer",
				"expires_in":   600,
				"id_token":     idToken,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{leafCert}}
	server.StartTLS()
	defer server.Close()
	issuer = server.URL

	// The headless completion reads the authorization code from a file instead
	// of the loopback callback, exactly like the documented --auth-code-file
	// / --no-browser path.
	authCodePath := filepath.Join(t.TempDir(), "auth-code.txt")
	if err := os.WriteFile(authCodePath, []byte("ca-integration-code\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	// Trust is provided only through the --ca gate (additive managed-CA root
	// pool). No injected http.Client: the managed CA is the sole verification
	// path.
	cfg := Config{
		Issuer:       issuer,
		ClientID:     "baseharbor-cli",
		Scopes:       []string{"openid", "profile"},
		CallbackPort: 0, // random loopback port (browser not used headless)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate test OIDC config: %v", err)
	}

	out := &synchronizedBuffer{}
	resultCh := make(chan error, 1)
	go func() {
		_, err := Login(context.Background(), "local", "test", cfg, out, TrustOptions{
			ExplicitCaFile: caPath,
			AuthCodeFile:   authCodePath,
			NoBrowser:      true, // --no-browser selects the headless path
		})
		resultCh <- err
	}()

	var authURL string
	deadline := time.Now().Add(5 * time.Second)
	for authURL == "" && time.Now().Before(deadline) {
		for _, field := range strings.Fields(out.String()) {
			if strings.HasPrefix(field, issuer+"/authorize?") {
				authURL = field
				break
			}
		}
		if authURL == "" {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if authURL == "" {
		t.Fatalf("authorization URL was not emitted: %q", out.String())
	}

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" {
		t.Fatalf("combined production login does not use PKCE S256: %s", authURL)
	}
	if query.Get("state") == "" {
		t.Fatalf("combined production login missing state: %s", authURL)
	}

	if err := <-resultCh; err != nil {
		t.Fatalf("combined CA-signed production login failed: %v", err)
	}

	verifier := <-verifierCh
	challengeDigest := sha256.Sum256([]byte(verifier))
	got := base64.RawURLEncoding.EncodeToString(challengeDigest[:])
	if got != query.Get("code_challenge") {
		t.Fatalf("combined PKCE verifier does not match challenge: got %q want %q", got, query.Get("code_challenge"))
	}

	session, err := LoadSession("local", "test")
	if err != nil {
		t.Fatalf("load persisted operator session: %v", err)
	}
	if session.Subject != "ca-operator" {
		t.Fatalf("combined login subject = %q", session.Subject)
	}
	if session.Issuer != issuer {
		t.Fatalf("combined login issuer = %q", session.Issuer)
	}
}

// TestLoginFixedCallbackPortAssertsBoundRedirectURI asserts that a configured
// fixed callback port is honored: the printed loopback callback redirect URI
// carries the fixed port and matches what the production Login bound. This
// complements the combined production path above, which uses a random port.
func TestLoginFixedCallbackPortAssertsBoundRedirectURI(t *testing.T) {
	idKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	caPath := filepath.Join(dir, "managed-ca.pem")
	leafCertPEM, leafKeyPEM := buildManagedCALeaf(t, caPath)
	leafCert, err := tls.X509KeyPair(leafCertPEM, leafKeyPEM)
	if err != nil {
		t.Fatalf("build leaf tls pair: %v", err)
	}

	var issuer string
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeOperatorAuthJSON(t, w, map[string]any{
				"issuer":                 issuer,
				"authorization_endpoint": issuer + "/authorize",
				"token_endpoint":         issuer + "/token",
				"jwks_uri":               issuer + "/keys",
			})
		case "/keys":
			writeOperatorAuthJSON(t, w, map[string]any{
				"keys": []map[string]any{{
					"kty": "RSA",
					"kid": "operator-test",
					"use": "sig",
					"alg": "RS256",
					"n":   base64.RawURLEncoding.EncodeToString(idKey.PublicKey.N.Bytes()),
					"e":   "AQAB",
				}},
			})
		case "/token":
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			now := time.Now()
			idToken := signOperatorAuthIDToken(t, idKey, map[string]any{
				"iss": issuer,
				"sub": "ca-operator",
				"aud": "baseharbor-cli",
				"exp": now.Add(10 * time.Minute).Unix(),
				"iat": now.Add(-time.Minute).Unix(),
			})
			writeOperatorAuthJSON(t, w, map[string]any{
				"access_token": "opaque-access-token",
				"token_type":   "Bearer",
				"expires_in":   600,
				"id_token":     idToken,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{leafCert}}
	server.StartTLS()
	defer server.Close()
	issuer = server.URL

	authCodePath := filepath.Join(t.TempDir(), "auth-code.txt")
	if err := os.WriteFile(authCodePath, []byte("ca-integration-code\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	cfg := Config{
		Issuer:       issuer,
		ClientID:     "baseharbor-cli",
		Scopes:       []string{"openid", "profile"},
		CallbackPort: 18080, // fixed loopback callback port
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate test OIDC config: %v", err)
	}

	out := &synchronizedBuffer{}
	resultCh := make(chan error, 1)
	go func() {
		_, err := Login(context.Background(), "local", "test", cfg, out, TrustOptions{
			ExplicitCaFile: caPath,
			AuthCodeFile:   authCodePath,
			NoBrowser:      true,
		})
		resultCh <- err
	}()

	var printedCallback string
	deadline := time.Now().Add(5 * time.Second)
	for printedCallback == "" && time.Now().Before(deadline) {
		for _, line := range strings.Split(out.String(), "\n") {
			if strings.Contains(line, "Loopback callback bound to:") {
				printedCallback = strings.TrimSpace(strings.TrimPrefix(line, "Loopback callback bound to:"))
				break
			}
		}
		if printedCallback == "" {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if printedCallback == "" {
		t.Fatalf("fixed callback URL was not printed: %q", out.String())
	}
	if printedCallback != "http://127.0.0.1:18080/callback" {
		t.Fatalf("fixed callback URL = %q, want http://127.0.0.1:18080/callback", printedCallback)
	}

	if err := <-resultCh; err != nil {
		t.Fatalf("fixed-callback production login failed: %v", err)
	}
}
