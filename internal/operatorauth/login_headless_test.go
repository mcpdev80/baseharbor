package operatorauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// TestLoginHeadlessCompletionViaCodeFile drives the full operator login with a
// non-browser session: the authorization code is supplied through a file
// (--auth-code-file / AuthCodeFile) instead of the loopback callback. This is
// the documented headless path for SSH/TTY-less hosts.
func TestLoginHeadlessCompletionViaCodeFile(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
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
					"n":   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
					"e":   "AQAB",
				}},
			})
		case "/token":
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "headless-code" {
				http.Error(w, "unexpected authorization code exchange", http.StatusBadRequest)
				return
			}
			if r.Form.Get("code_verifier") == "" {
				http.Error(w, "missing PKCE verifier", http.StatusBadRequest)
				return
			}
			verifierCh <- r.Form.Get("code_verifier")
			now := time.Now()
			idToken := signOperatorAuthIDToken(t, key, map[string]any{
				"iss": issuer,
				"sub": "headless-operator",
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
	server.StartTLS()
	defer server.Close()
	issuer = server.URL

	// The headless session reads the authorization code from a file rather than
	// completing the loopback callback.
	authCodePath := filepath.Join(t.TempDir(), "auth-code.txt")
	if err := os.WriteFile(authCodePath, []byte("headless-code\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ctx = oidc.ClientContext(ctx, server.Client())

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
		_, err := Login(ctx, "local", "test", cfg, out, TrustOptions{AuthCodeFile: authCodePath})
		resultCh <- err
	}()

	// The printed auth URL is what a non-browser operator pastes into the code
	// file after completing the IdP flow.
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
		t.Fatalf("headless authorization request does not use PKCE S256: %s", authURL)
	}
	if query.Get("state") == "" {
		t.Fatalf("headless authorization request missing state: %s", authURL)
	}

	if err := <-resultCh; err != nil {
		t.Fatalf("headless operator OIDC login: %v", err)
	}

	verifier := <-verifierCh
	challengeDigest := sha256.Sum256([]byte(verifier))
	got := base64.RawURLEncoding.EncodeToString(challengeDigest[:])
	if got != query.Get("code_challenge") {
		t.Fatalf("headless PKCE verifier does not match challenge: got %q want %q", got, query.Get("code_challenge"))
	}

	session, err := LoadSession("local", "test")
	if err != nil {
		t.Fatalf("load persisted operator session: %v", err)
	}
	if session.Subject != "headless-operator" {
		t.Fatalf("headless session subject = %q", session.Subject)
	}
}
