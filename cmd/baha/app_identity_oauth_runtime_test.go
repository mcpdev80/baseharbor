package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// Tokens and credentials stay only in memory; receipt messages contain outcomes.
// This executes the selected application's real authorization-code flow, not
// the admin password grant or a mocked issuer, for end-user authentication.
type identityOAuthRuntimeSession struct {
	client                                                                 *http.Client
	issuer, authorization, tokenEndpoint, userinfo, clientID, clientSecret string
	callback, username, password, subject, refreshToken                    string
}

type identityOAuthTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
}

func startIdentityOAuthRuntimeSession(t *testing.T, ctx context.Context, resolved resolvedApplication, files identityprovider.KeycloakFiles) *identityOAuthRuntimeSession {
	t.Helper()
	binding := filepath.Join(application.RuntimeFilesFor(resolved.Store, resolved.Manifest).Bindings, application.IdentityBindingName)
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(binding, name))
		if err != nil || len(bytes.TrimSpace(data)) == 0 {
			t.Fatal("real OAuth binding is missing")
		}
		return strings.TrimSpace(string(data))
	}
	issuer := read("oidc.issuer")
	parsed, err := url.Parse(issuer)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		t.Fatal("OAuth issuer is not canonical HTTPS")
	}
	ca, err := os.ReadFile(filepath.Join(binding, "ca.crt"))
	roots := x509.NewCertPool()
	if err != nil || !roots.AppendCertsFromPEM(ca) {
		t.Fatal("OAuth binding trust is invalid")
	}
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(address)
		if err != nil || host != parsed.Hostname() {
			return nil, &identityOAuthFixtureError{}
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
	}}
	t.Cleanup(transport.CloseIdleConnections)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal("OAuth cookie store unavailable")
	}
	session := &identityOAuthRuntimeSession{issuer: issuer, authorization: read("oidc.authorization-endpoint"), tokenEndpoint: read("oidc.token-endpoint"), userinfo: read("oidc.userinfo-endpoint"), clientID: read("oidc.client-id"), clientSecret: read("client-secret"), callback: "https://oauth-fixture.invalid/oauth/callback", username: "native-oauth-user", password: identityOAuthRandom(t)}
	session.client = &http.Client{Transport: transport, Jar: jar, Timeout: 15 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return &identityOAuthFixtureError{}
		}
		if req.URL.Host != parsed.Host {
			return http.ErrUseLastResponse
		}
		return nil
	}}
	// The manifest declares this callback through the actual public workload
	// origin. It is intercepted before connecting; no test callback listener can
	// accidentally exchange or log the returned authorization code.
	if len(resolved.Manifest.Identity.CallbackPaths) != 1 {
		t.Fatal("OAuth fixture callback intent is missing")
	}
	// The renderer supplies the actual registered origin below, not a test-only
	// client or a relaxed wildcard in Keycloak.
	session.callback = identityOAuthFixtureCallback(t, resolved)
	// Native retirement inventory intentionally contains only resource identity.
	// Load the existing complete access projection without materializing state.
	adminFiles, err := identityprovider.ExistingKeycloakFilesAt(resolved.Manifest, resolved.TargetStateRoot, resolved.Target.Name)
	if err != nil || adminFiles.Dir != files.Dir || adminFiles.Project != files.Project {
		t.Fatal("OAuth administration does not match the observed existing provider owner")
	}
	provisionIdentityOAuthFixtureUser(t, ctx, adminFiles, issuer, resolved.Manifest.Name, session.username, session.password)
	session.login(t, ctx, true)
	session.login(t, ctx, false)
	return session
}

type identityOAuthFixtureError struct{}

func (*identityOAuthFixtureError) Error() string { return "OAuth fixture request boundary failed" }

func identityOAuthRandom(t *testing.T) string {
	t.Helper()
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		t.Fatal("OAuth entropy unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

func identityOAuthFixtureCallback(t *testing.T, resolved resolvedApplication) string {
	t.Helper()
	host, err := devaccess.ApplicationHost(resolved.Target.Name, resolved.Manifest.Name, "api")
	if err != nil {
		t.Fatal("OAuth callback origin invalid")
	}
	// This is the public exposure name authored by the bounded native fixture.
	return devgateway.URLForTarget(resolved.Target.Name, host) + "/oauth/callback"
}

func provisionIdentityOAuthFixtureUser(t *testing.T, ctx context.Context, files identityprovider.KeycloakFiles, issuer, applicationName, username, password string) {
	t.Helper()
	values := map[string]string{}
	for index, path := range []string{files.Env, filepath.Join(files.Dir, "active-admin.env")} {
		data, err := os.ReadFile(path)
		if index == 1 && os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal("isolated OAuth admin credential unavailable")
		}
		for _, line := range strings.Split(string(data), "\n") {
			if key, value, ok := strings.Cut(line, "="); ok {
				values[key] = value
			}
		}
	}
	client, err := serviceaccess.NewHTTPClient(files.AdminAccess.Material, false)
	if err != nil {
		t.Fatal("isolated OAuth admin trust unavailable")
	}
	if transport, ok := client.Transport.(*http.Transport); ok {
		t.Cleanup(transport.CloseIdleConnections)
	}
	form := url.Values{"grant_type": {"password"}, "client_id": {"admin-cli"}, "username": {values["BASEHARBOR_KEYCLOAK_ADMIN_USER"]}, "password": {values["BASEHARBOR_KEYCLOAK_ADMIN_PASSWORD"]}}
	response := identityOAuthRequest(t, ctx, client, http.MethodPost, files.AdminURL+"/realms/master/protocol/openid-connect/token", strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", "")
	var tokens identityOAuthTokens
	identityOAuthDecode(t, response, http.StatusOK, &tokens)
	if tokens.AccessToken == "" {
		t.Fatal("isolated OAuth admin authentication failed")
	}
	parsed, err := url.Parse(issuer)
	if err != nil || !strings.HasPrefix(parsed.Path, "/realms/") {
		t.Fatal("application OAuth realm unavailable")
	}
	base := files.AdminURL + "/admin" + parsed.Path
	response = identityOAuthRequest(t, ctx, client, http.MethodGet, base, nil, "", tokens.AccessToken)
	var realm struct {
		Attributes map[string]string `json:"attributes"`
	}
	identityOAuthDecode(t, response, http.StatusOK, &realm)
	if realm.Attributes["baseharbor.owner"] != "baseharbor" || realm.Attributes["baseharbor.application"] != applicationName || realm.Attributes["baseharbor.environment"] != "dev" {
		t.Fatal("OAuth fixture cannot create a user in an unowned realm")
	}
	body, err := json.Marshal(map[string]any{"username": username, "firstName": "Native", "lastName": "Fixture", "email": "native-oauth@example.invalid", "emailVerified": true, "enabled": true, "requiredActions": []string{}, "credentials": []map[string]any{{"type": "password", "value": password, "temporary": false}}})
	if err != nil {
		t.Fatal("OAuth user fixture invalid")
	}
	response = identityOAuthRequest(t, ctx, client, http.MethodPost, base+"/users", bytes.NewReader(body), "application/json", tokens.AccessToken)
	identityOAuthDecode(t, response, http.StatusCreated, nil)
}

func identityOAuthRequest(t *testing.T, ctx context.Context, client *http.Client, method, endpoint string, body io.Reader, contentType, bearer string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		t.Fatal("OAuth request invalid")
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("OAuth request failed; sensitive request detail withheld")
	}
	return response
}

func identityOAuthDecode(t *testing.T, response *http.Response, expected int, result any) {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode != expected {
		t.Fatalf("OAuth endpoint status=%d expected=%d", response.StatusCode, expected)
	}
	if result != nil && json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result) != nil {
		t.Fatal("OAuth response invalid")
	}
}

var identityOAuthFormPattern = regexp.MustCompile(`(?is)<form\b[^>]*>`)
var identityOAuthAttributePattern = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9_-]*)\s*=\s*["']([^"']*)["']`)

func (s *identityOAuthRuntimeSession) login(t *testing.T, ctx context.Context, rejectPKCE bool) {
	t.Helper()
	verifier, state, nonce := identityOAuthRandom(t), identityOAuthRandom(t), identityOAuthRandom(t)
	digest := sha256.Sum256([]byte(verifier))
	query := url.Values{"client_id": {s.clientID}, "redirect_uri": {s.callback}, "response_type": {"code"}, "scope": {"openid profile email"}, "prompt": {"login"}, "state": {state}, "nonce": {nonce}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(digest[:])}}
	response := identityOAuthRequest(t, ctx, s.client, http.MethodGet, s.authorization+"?"+query.Encode(), nil, "", "")
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("OAuth login page status=%d", response.StatusCode)
	}
	page, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if err != nil {
		t.Fatal("OAuth login form unavailable")
	}
	action := ""
	for _, form := range identityOAuthFormPattern.FindAllString(string(page), -1) {
		attrs := map[string]string{}
		for _, match := range identityOAuthAttributePattern.FindAllStringSubmatch(form, -1) {
			attrs[strings.ToLower(match[1])] = html.UnescapeString(match[2])
		}
		if attrs["id"] == "kc-form-login" {
			action = attrs["action"]
		}
	}
	parsed, err := url.Parse(action)
	issuer, _ := url.Parse(s.issuer)
	if err != nil || parsed.Scheme != "https" || parsed.Host != issuer.Host || !strings.HasPrefix(parsed.Path, "/realms/") {
		t.Fatal("OAuth login action is outside selected issuer")
	}
	form := url.Values{"username": {s.username}, "password": {s.password}, "credentialId": {""}}
	response = identityOAuthRequest(t, ctx, s.client, http.MethodPost, action, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", "")
	defer response.Body.Close()
	if response.StatusCode != http.StatusFound && response.StatusCode != http.StatusSeeOther {
		t.Fatalf("OAuth user login status=%d", response.StatusCode)
	}
	location, err := response.Location()
	callback, _ := url.Parse(s.callback)
	if err != nil || location.Scheme != callback.Scheme || location.Host != callback.Host || location.Path != callback.Path || location.Query().Get("state") != state || location.Query().Get("error") != "" || location.Query().Get("code") == "" {
		t.Fatal("OAuth callback or state verification failed")
	}
	form = url.Values{"grant_type": {"authorization_code"}, "code": {location.Query().Get("code")}, "redirect_uri": {s.callback}, "client_id": {s.clientID}, "client_secret": {s.clientSecret}, "code_verifier": {verifier}}
	if rejectPKCE {
		form.Set("code_verifier", identityOAuthRandom(t))
	}
	response = identityOAuthRequest(t, ctx, s.client, http.MethodPost, s.tokenEndpoint, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", "")
	if rejectPKCE {
		identityOAuthDecode(t, response, http.StatusBadRequest, nil)
		t.Log("actual issuer rejects a mismatched S256 PKCE verifier")
		return
	}
	var tokens identityOAuthTokens
	identityOAuthDecode(t, response, http.StatusOK, &tokens)
	s.verifyTokens(t, ctx, tokens, nonce)
	t.Log("actual end-user authorization-code/S256 PKCE login and signed ID-token verification PASS")
}

func (s *identityOAuthRuntimeSession) verifyTokens(t *testing.T, ctx context.Context, tokens identityOAuthTokens, nonce string) {
	t.Helper()
	if tokens.AccessToken == "" || tokens.RefreshToken == "" || tokens.IDToken == "" {
		t.Fatal("OAuth response lacks required token material")
	}
	verifyCtx := oidc.ClientContext(ctx, s.client)
	provider, err := oidc.NewProvider(verifyCtx, s.issuer)
	if err != nil {
		t.Fatal("actual OAuth issuer discovery failed")
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: s.clientID}).Verify(verifyCtx, tokens.IDToken)
	if err != nil || idToken.Subject == "" || (nonce != "" && idToken.Nonce != nonce) {
		t.Fatal("OAuth signed ID token, subject or nonce invalid")
	}
	if s.subject != "" && s.subject != idToken.Subject {
		t.Fatal("re-enabled Identity replaced retained user identity")
	}
	response := identityOAuthRequest(t, ctx, s.client, http.MethodGet, s.userinfo, nil, "", tokens.AccessToken)
	var user struct {
		Subject string `json:"sub"`
	}
	identityOAuthDecode(t, response, http.StatusOK, &user)
	if user.Subject != idToken.Subject {
		t.Fatal("OAuth actual userinfo differs from signed identity")
	}
	s.subject, s.refreshToken = idToken.Subject, tokens.RefreshToken
}

func (s *identityOAuthRuntimeSession) refresh(t *testing.T, ctx context.Context) {
	t.Helper()
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {s.refreshToken}, "client_id": {s.clientID}, "client_secret": {s.clientSecret}}
	response := identityOAuthRequest(t, ctx, s.client, http.MethodPost, s.tokenEndpoint, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", "")
	var tokens identityOAuthTokens
	identityOAuthDecode(t, response, http.StatusOK, &tokens)
	s.verifyTokens(t, ctx, tokens, "")
	t.Log("actual retained OAuth session refresh and userinfo PASS")
}

func (s *identityOAuthRuntimeSession) assertUnavailable(t *testing.T, ctx context.Context) {
	t.Helper()
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {s.refreshToken}, "client_id": {s.clientID}, "client_secret": {s.clientSecret}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal("disabled OAuth probe invalid")
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := s.client.Do(request)
	if err == nil {
		defer response.Body.Close()
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			t.Fatal("disabled Identity still refreshes end-user sessions")
		}
	}
	t.Log("disabled actual Identity no longer accepts session refresh; retained token alone cannot renew access")
}
