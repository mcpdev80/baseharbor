package operatorauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

func TestConfigValidateRejectsInsecureOrIncompleteOIDC(t *testing.T) {
	cases := []Config{
		{Issuer: "http://id.example.test", ClientID: "baha"},
		{Issuer: "https://id.example.test", ClientID: ""},
		{Issuer: "https://id.example.test?token=x", ClientID: "baha"},
		{Issuer: "https://id.example.test", ClientID: "baha", Scopes: []string{"openid profile"}},
	}
	for _, cfg := range cases {
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected invalid config to fail: %#v", cfg)
		}
	}
}

func TestManagedEnvironmentKeepsDevTrustedLocal(t *testing.T) {
	if ManagedEnvironment("dev") || ManagedEnvironment("development") {
		t.Fatal("development must stay trusted-local")
	}
	if !ManagedEnvironment("test") || !ManagedEnvironment("prod") {
		t.Fatal("test/prod must require managed operator authentication")
	}
}

func TestSessionIsTargetAndEnvironmentScopedAndExpires(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	session := Session{
		Target:      "local",
		Environment: "test",
		Issuer:      "https://id.example.test",
		Subject:     "operator-1",
		ClientID:    "baseharbor-cli",
		IDToken:     "opaque-test-token",
		ExpiresAt:   time.Now().Add(5 * time.Minute),
	}
	if err := SaveSession(session); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(runtimeDir, "baseharbor", "operator-local-test.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("session mode=%#o", info.Mode().Perm())
	}
	if _, err := LoadSession("local", "prod"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cross-environment session unexpectedly resolved: %v", err)
	}
	if _, err := LoadSession("other", "test"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cross-target session unexpectedly resolved: %v", err)
	}

	cfg := Config{Issuer: session.Issuer, ClientID: session.ClientID}
	observed, err := ObserveSession("local", "test", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !observed.Present || !observed.Valid || observed.Subject != session.Subject {
		t.Fatalf("observation=%#v", observed)
	}

	session.ExpiresAt = time.Now().Add(10 * time.Second)
	if err := SaveSession(session); err == nil {
		t.Fatal("near-expired session must not be persisted")
	}
}

type synchronizedBuffer struct {
	mu sync.Mutex
	s  string
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.s += string(p)
	return len(p), nil
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.s
}

func TestLoginCompletesOIDCAuthorizationCodePKCEFlow(t *testing.T) {
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
			if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "operator-code" {
				http.Error(w, "unexpected authorization code exchange", http.StatusBadRequest)
				return
			}
			verifier := r.Form.Get("code_verifier")
			if verifier == "" {
				http.Error(w, "missing PKCE verifier", http.StatusBadRequest)
				return
			}
			verifierCh <- verifier
			now := time.Now()
			idToken := signOperatorAuthIDToken(t, key, map[string]any{
				"iss": issuer,
				"sub": "operator-1",
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

	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ctx = oidc.ClientContext(ctx, server.Client())

	cfg := Config{
		Issuer:   issuer,
		ClientID: "baseharbor-cli",
		Scopes:   []string{"openid", "profile"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate test OIDC config: %v", err)
	}

	out := &synchronizedBuffer{}
	type loginResult struct {
		principalSubject string
		err              error
	}
	resultCh := make(chan loginResult, 1)
	go func() {
		principal, err := Login(ctx, "local", "test", cfg, out)
		result := loginResult{err: err}
		if principal != nil {
			result.principalSubject = principal.Subject
		}
		resultCh <- result
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
	if query.Get("client_id") != cfg.ClientID || query.Get("response_type") != "code" {
		t.Fatalf("unexpected authorization request: %s", authURL)
	}
	if query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" {
		t.Fatalf("authorization request does not use PKCE S256: %s", authURL)
	}
	if query.Get("state") == "" || query.Get("redirect_uri") == "" {
		t.Fatalf("authorization request is missing state/callback: %s", authURL)
	}

	callbackURL := query.Get("redirect_uri") + "?state=" + url.QueryEscape(query.Get("state")) + "&code=operator-code"
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("complete local OIDC callback: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("callback status = %s", resp.Status)
	}

	result := <-resultCh
	if result.err != nil {
		t.Fatalf("operator OIDC login: %v", result.err)
	}
	if result.principalSubject != "operator-1" {
		t.Fatalf("principal subject = %q", result.principalSubject)
	}

	verifier := <-verifierCh
	challengeDigest := sha256.Sum256([]byte(verifier))
	if got := base64.RawURLEncoding.EncodeToString(challengeDigest[:]); got != query.Get("code_challenge") {
		t.Fatalf("PKCE verifier does not match challenge: got %q want %q", got, query.Get("code_challenge"))
	}

	session, err := LoadSession("local", "test")
	if err != nil {
		t.Fatalf("load persisted operator session: %v", err)
	}
	if session.Subject != "operator-1" || session.Issuer != issuer || session.ClientID != cfg.ClientID {
		t.Fatalf("unexpected persisted session: %#v", session)
	}
	info, err := os.Stat(filepath.Join(runtimeDir, "baseharbor", "operator-local-test.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("session mode=%#o", info.Mode().Perm())
	}
}

func writeOperatorAuthJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func signOperatorAuthIDToken(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	header, err := json.Marshal(map[string]any{"alg": "RS256", "typ": "JWT", "kid": "operator-test"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	encode := base64.RawURLEncoding.EncodeToString
	signingInput := encode(header) + "." + encode(payload)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + encode(signature)
}
