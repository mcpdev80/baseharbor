package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

func TestParseOperatorLoginArgs(t *testing.T) {
	args, init, err := parseOperatorLoginArgs([]string{"-e", "dev", "--init-tenant"})
	if err != nil || !init || !reflect.DeepEqual(args, []string{"-e", "dev"}) {
		t.Fatalf("parsed login flags = %v, %t, %v", args, init, err)
	}
	args, init, err = parseOperatorLoginArgs([]string{"-e", "test"})
	if err != nil || init || !reflect.DeepEqual(args, []string{"-e", "test"}) {
		t.Fatalf("ordinary login changed = %v, %t, %v", args, init, err)
	}
	if _, _, err = parseOperatorLoginArgs([]string{"--init-tenant", "--init-tenant"}); err == nil {
		t.Fatal("duplicate first-owner authorization must fail")
	}
}

func TestInitialOperatorBootstrapRejectsRemoteTarget(t *testing.T) {
	target := deployment.ResolvedTarget{AccessProvider: "baseharbor-node-connector"}
	if _, err := initialOperatorLocalCoreFiles(context.Background(), target); !errors.Is(err, errInitialOperatorLocalOnly) {
		t.Fatalf("remote target must not bootstrap initial tenant: %v", err)
	}
}

func TestInitialTenantDefaultAndCustomOptions(t *testing.T) {
	args, init, slug, name, err := parseInitialTenantLoginArgs([]string{"--init-tenant", "-e", "dev"})
	if err != nil || !init || slug != "baseharbor" || name != "BaseHarbor" || !reflect.DeepEqual(args, []string{"-e", "dev"}) {
		t.Fatalf("default login options: %v %t %s %s %v", args, init, slug, name, err)
	}
	args, init, slug, name, err = parseInitialTenantLoginArgs([]string{"-e", "prod", "--init-tenant", "--tenant-slug", "acme", "--tenant-name", "Acme GmbH"})
	if err != nil || !init || slug != "acme" || name != "Acme GmbH" || !reflect.DeepEqual(args, []string{"-e", "prod"}) {
		t.Fatalf("custom login options: %v %t %s %s %v", args, init, slug, name, err)
	}
	for _, invalid := range [][]string{{"--tenant-slug", "acme"}, {"--init-tenant", "--tenant-name"}, {"--init-tenant", "--tenant-slug", "a", "--tenant-slug", "b"}} {
		if _, _, _, _, err := parseInitialTenantLoginArgs(invalid); err == nil {
			t.Fatalf("invalid first-tenant options accepted: %v", invalid)
		}
	}
}

// TestParseLoginTrustArgsNoBrowserAcceptsHeadlessFlag proves the documented
// --no-browser companion flag is accepted by generic argument parsing (it no
// longer errors) AND is captured into a NoBrowser result so the CLI selects the
// headless path instead of launching a browser. This is the CLI-level
// regression the production command rejected before the fix.
func TestParseLoginTrustArgsNoBrowserAcceptsHeadlessFlag(t *testing.T) {
	remaining, ca, authCode, noBrowser, err := parseLoginTrustArgs([]string{"-e", "test", "--no-browser"})
	if err != nil {
		t.Fatalf("--no-browser must not error at the CLI: %v", err)
	}
	if noBrowser != true {
		t.Fatalf("--no-browser must be captured as NoBrowser=true, got %t", noBrowser)
	}
	// The remaining args keep only the environment flag for
	// extractApplicationEnvironment. --no-browser is consumed here (not echoed
	// back) so it never reaches the generic leftover check.
	if !reflect.DeepEqual(remaining, []string{"-e", "test"}) {
		t.Fatalf("--no-browser must be consumed and absent from remaining, got %v", remaining)
	}

	// A --ca file alongside --no-browser still parses and captures both values.
	remaining, ca, authCode, noBrowser, err = parseLoginTrustArgs([]string{"-e", "test", "--ca", "/tmp/ca.pem", "--auth-code-file", "/tmp/code.txt", "--no-browser"})
	if err != nil {
		t.Fatalf("--ca/--auth-code-file/--no-browser must parse: %v", err)
	}
	if !noBrowser || ca != "/tmp/ca.pem" || authCode != "/tmp/code.txt" {
		t.Fatalf("combined trust flags not captured: ca=%q authCode=%q noBrowser=%t", ca, authCode, noBrowser)
	}
	// Even alongside --ca/--auth-code-file, --no-browser is consumed, not echoed.
	if !reflect.DeepEqual(remaining, []string{"-e", "test"}) {
		t.Fatalf("--no-browser must be consumed in the combined case, got %v", remaining)
	}
}

// TestOperatorLoginCommandNoBrowserReachesHeadlessPath is the command-level
// regression proving the documented `baha login -e managed --no-browser`
// invocation reaches the real OIDC login path: --no-browser is consumed by
// parseLoginTrustArgs (never rejected by the generic leftover check) AND
// forwarded into operatorauth.Login as NoBrowser=true, which selects the
// headless PKCE completion instead of launching a browser. Unlike a dev
// trusted-local run the command must NOT return the "no login required"
// early-return; it must drive operatorauth.Login to completion over an
// in-process OIDC server whose leaf is signed only by the managed test CA.
//
// The assertions are counterfactual: they prove Login was actually reached by
// observing server-side and persistence side effects that cannot occur on any
// early return. Removing the --no-browser consumption (cmd/baha/operator_auth.go
// re-adding --no-browser to the leftover args) or dropping NoBrowser on the
// Login call makes the token exchange 400 and the test fail; an alternate silent
// early return produces neither the exchange nor the persisted session and also
// fails.
func TestOperatorLoginCommandNoBrowserReachesHeadlessPath(t *testing.T) {
	// Managed environment (never the trusted-local early return) plus the
	// explicit operator OIDC config the command resolves via ConfigFromEnv.
	t.Setenv(operatorauth.EnvIssuer, "")
	t.Setenv(operatorauth.EnvClientID, "")
	t.Setenv(operatorauth.EnvScopes, "")
	t.Setenv(operatorauth.EnvCallbackPort, "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	issuer := startManagedOperatorOIDCServer(t)
	t.Setenv(operatorauth.EnvIssuer, issuer)
	t.Setenv(operatorauth.EnvClientID, "baseharbor-cli")
	t.Setenv(operatorauth.EnvScopes, "openid,profile")
	t.Setenv(operatorauth.EnvCallbackPort, "0")
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	ctx := operatorauth.WithEnforcement(context.Background())

	// A local, local-access Target so effectiveTarget resolves "local" and the
	// managed path reaches operatorauth.Login. XDG_RUNTIME_DIR roots the
	// operator session persistence the command writes after login.
	resolvedTarget := configureTestTarget(t)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	// The documented headless login: -e managed + --no-browser + --auth-code-file.
	// With the fix, --no-browser is consumed and forwarded; the command must NOT
	// return the trusted-local no-login early-return and must complete
	// operatorauth.Login headlessly through the managed test CA.
	authCodePath := filepath.Join(t.TempDir(), "auth-code.txt")
	if err := os.WriteFile(authCodePath, []byte("managed-headless-code\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	loginArgs := []string{"-e", "managed", "--no-browser", "--auth-code-file", authCodePath}
	err := operatorLoginCommand().Run(ctx, loginArgs, &out, &errOut)
	if err != nil {
		t.Fatalf("--no-browser managed login must reach operatorauth.Login without a leftover-check rejection: %v", err)
	}
	if strings.Contains(out.String(), "trusted local operator mode") {
		t.Fatalf("managed login must not take the trusted-local early return, got: %q", out.String())
	}

	// (1) Prove Login was reached, not merely that the early-return string is
	// absent. operatorauth.Login exchanges the auth code for an ID token, so a
	// successful run must (a) have recorded the server-side token exchange and
	// (b) have persisted an authenticated principal for the local/target. Either
	// witness failing means Login never completed and the test must fail.
	if !operatorOIDCExchangesCompleted() {
		t.Fatalf("--no-browser managed login must complete the OIDC token exchange to prove Login was reached")
	}
	observed, perr := operatorauth.ObserveSession(resolvedTarget.Name, "managed", mustResolveOperatorAuthBoundaryConfig(t, "managed"))
	if perr != nil {
		t.Fatalf("--no-browser managed login must persist an authenticated principal after Login without error, got: %v", perr)
	}
	if !observed.Valid {
		t.Fatalf("--no-browser managed login must persist a valid authenticated principal after Login; observed=%+v", observed)
	}

	// (1b) Counterfactual note (kept in the source so the witnesses are not
	// misread as cosmetic): a silent early return that skipped
	// operatorauth.Login would complete neither the token exchange nor the
	// session persistence, so both assertions below would fail. They only pass
	// when Login actually ran to completion, which is exactly what this
	// command-level regression is meant to prove.

	// (2) Browser isolation: the --no-browser flag must suppress the browser
	// launch that operatorauth.Login performs for a non-headless flow. Clear any
	// inherited BASEHARBOR_NO_BROWSER override, then replace the Linux CI
	// xdg-open launcher with a temporary marker script on a temp PATH so a real
	// launch is observable. Without the NoBrowser forward (mutation at
	// cmd/baha/operator_auth.go:64) the command would exec the marker and this
	// assertion fails; with the flag it must not.
	clearInheritedNoBrowser(t)
	observer := installFakeXdgOpen(t)
	defer observer.restore()

	// A plain, non-enforced context: Login persists the session directly, so no
	// interactive confirmation is required. The target uses the same resolved
	// target name the command path uses so the persisted session matches the
	// ObserveSession witness above.
	_, lerr := operatorauth.Login(context.Background(), resolvedTarget.Name, "managed", mustResolveOperatorAuthBoundaryConfig(t, "managed"), &bytes.Buffer{}, operatorauth.TrustOptions{
		AuthCodeFile: authCodePath,
		NoBrowser:    true,
	})
	if lerr != nil {
		t.Fatalf("operatorauth.Login with NoBrowser=true must not error: %v", lerr)
	}
	if marker := observer.anyLaunch(); marker != "" {
		t.Fatalf("--no-browser=true must not launch any browser; observed marker invocation: %q", marker)
	}

	// (3) unknown-leftover flag control remains alive after the --no-browser
	// consumption fix: --no-browser specifically was consumed and forwarded, not
	// blindly dropped before the login path.
	err = operatorLoginCommand().Run(ctx, []string{"-e", "managed", "--bogus-flag"}, &out, &errOut)
	if err == nil || !strings.Contains(err.Error(), "accepts only") {
		t.Fatalf("unknown leftover flag must still be rejected, got err=%v", err)
	}
}

// sanitizeObserverName reduces a Go test name to a filesystem-safe token.
func sanitizeObserverName(name string) string {
	replacer := strings.NewReplacer("/", "_", " ", "_", ":", "_")
	return replacer.Replace(name)
}

// clearInheritedNoBrowser removes any inherited BASEHARBOR_NO_BROWSER override so
// the test observes the command's own flag handling rather than a pre-set
// headless marker. The mutation that drops NoBrowser on the operatorauth.Login
// call (cmd/baha/operator_auth.go:64) is only observable when no external
// override forces the headless path first.
func clearInheritedNoBrowser(t *testing.T) {
	t.Helper()
	// unset removes a possibly-inherited value from the child process view and
	// returns its prior value for the witness below.
	_ = os.Unsetenv("BASEHARBOR_NO_BROWSER")
}

// fakeXdgObserver watches for any browser launch on a temp PATH by replacing the
// Linux CI xdg-open launcher with a marker script. Any invocation of the marker
// proves a browser was attempted; a launch that reaches the marker is a failed
// suppression and is reported by anyLaunch.
type fakeXdgObserver struct {
	binDir     string
	markerPath string
	runs       string
}

func (o *fakeXdgObserver) anyLaunch() string {
	matches, _ := filepath.Glob(filepath.Join(o.runs, "launch-*"))
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}

func (o *fakeXdgObserver) restore() {
	_ = os.Unsetenv("PATH")
	_ = os.RemoveAll(o.binDir)
	_ = os.RemoveAll(o.runs)
}

// installFakeXdgOpen builds a temp bin directory containing a marker xdg-open
// script, prepends it to PATH for this process, and returns an observer. The
// marker records its target URL into a uniquely-named file in a known directory
// so the parent process can inspect the launch after the child execs it. The
// observer directory lives under /var/tmp (not the space-constrained /tmp) so
// the marker can reliably record the launch. Returns an empty anyLaunch result
// when the headless path never execs the marker.
func installFakeXdgOpen(t *testing.T) *fakeXdgObserver {
	t.Helper()
	base := "/var/tmp/baha-browser-observer-" + sanitizeObserverName(t.Name())
	_ = os.MkdirAll(base, 0o755)
	binDir := filepath.Join(base, "bin")
	runs := filepath.Join(base, "runs")
	_ = os.MkdirAll(binDir, 0o755)
	_ = os.MkdirAll(runs, 0o755)
	markerPath := filepath.Join(binDir, "xdg-open")
	// xdg-open is only launched on Linux in openBrowser; the marker records the
	// target URL and exits 0 so the ignored launch error does not propagate. The
	// PATH prepend makes exec.Command("xdg-open", ...) resolve to this marker.
	markerScript := "#!/bin/bash\nmkdir -p \"" + runs + "\"\nprintf '%s\\n' \"$1\" > \"" + runs + "/launch-" + "$(date +%s%N).arg\"\nexit 0\n"
	if err := os.WriteFile(markerPath, []byte(markerScript), 0o755); err != nil {
		t.Fatalf("write fake xdg-open marker: %v", err)
	}
	prevPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+prevPath)
	return &fakeXdgObserver{binDir: binDir, markerPath: markerPath, runs: runs}
}

// mustResolveOperatorAuthBoundaryConfig builds a validated operator OIDC Config
// from the test's environment settings, used to drive operatorauth.Login
// directly for the browser-suppression counterfactual.
func mustResolveOperatorAuthBoundaryConfig(t *testing.T, environment string) operatorauth.Config {
	t.Helper()
	cfg, err := operatorauth.ConfigFromEnv()
	if err != nil {
		t.Fatalf("resolve operator auth config from env: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validated operator auth config: %v", err)
	}
	return cfg
}

// startManagedOperatorOIDCServer boots an in-process HTTPS OIDC provider whose
// leaf is signed only by a fresh managed test CA, and returns its issuer URL
// so the test can drive the headless PKCE completion through operatorauth.Login.
// This is the production trust path: discovery trusts the managed CA resolved
// through the --ca/EnvTrustFile gate, never an injected client.
func startManagedOperatorOIDCServer(t *testing.T) string {
	t.Helper()
	idKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate OIDC signing key: %v", err)
	}
	dir := t.TempDir()
	caPath := filepath.Join(dir, "managed-ca.pem")
	caCert, caKey := writeTestCA(t, caPath)
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
	leafCert, err := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: leafKeyPEM}))
	if err != nil {
		t.Fatalf("build leaf tls pair: %v", err)
	}
	issuer := ""
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeOIDCResponse(w, map[string]any{
				"issuer":                 issuer,
				"authorization_endpoint": issuer + "/authorize",
				"token_endpoint":         issuer + "/token",
				"jwks_uri":               issuer + "/keys",
			})
		case "/keys":
			writeOIDCResponse(w, map[string]any{
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
			if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "managed-headless-code" {
				http.Error(w, "unexpected authorization code exchange", http.StatusBadRequest)
				return
			}
			// Record the server-side token exchange. This is the counterfactual
			// witness that operatorauth.Login completed the PKCE exchange and was
			// therefore reached; a silent early return produces no exchange.
			operatorOIDCExchanges.Add(1)
			if r.Form.Get("code_verifier") == "" {
				http.Error(w, "missing PKCE verifier", http.StatusBadRequest)
				return
			}
			now := time.Now()
			idToken := signTestIDToken(t, idKey, map[string]any{
				"iss": issuer,
				"sub": "managed-headless-operator",
				"aud": "baseharbor-cli",
				"exp": now.Add(10 * time.Minute).Unix(),
				"iat": now.Add(-time.Minute).Unix(),
			})
			writeOIDCResponse(w, map[string]any{
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
	t.Cleanup(server.Close)
	issuer = server.URL

	// The headless completion reads the authorization code from a file, exactly
	// like the documented --auth-code-file / --no-browser path. Point the
	// discovery trust gate at the managed test CA so Login trusts the issuer
	// through the same --ca/managed-CA path the CLI uses in production, and feed
	// the auth code through the same env var the command resolves via
	// ConfigFromEnv so the headless flow completes instead of blocking.
	authCodePath := filepath.Join(t.TempDir(), "auth-code.txt")
	if err := os.WriteFile(authCodePath, []byte("managed-headless-code\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(operatorauth.EnvTrustFile, caPath)
	t.Setenv("BASEHARBOR_OPERATOR_OIDC_AUTH_CODE_FILE", authCodePath)
	return issuer
}

// writeTestCA mints a self-signed managed test CA written to caPath and returns
// the parsed certificate and its private key for signing the OIDC server leaf.
func writeTestCA(t *testing.T, caPath string) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate test CA key: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "baseharbor-managed-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("generate test CA certificate: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse test CA certificate: %v", err)
	}
	pemData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	if err := os.WriteFile(caPath, pemData, 0o600); err != nil {
		t.Fatalf("write test CA file %s: %v", caPath, err)
	}
	return caCert, caKey
}

// writeOIDCResponse writes a JSON object to an OIDC provider response with the
// application/json Content-Type the oauth2 library requires.
func writeOIDCResponse(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, value)
}

// operatorOIDCExchanges tracks whether the in-process managed OIDC server has
// completed one authorization-code/token exchange. It is the server-side witness
// that operatorauth.Login reached the exchange path; a test-local atomic
// counter survives the httptest server's closure so the command-level test can
// assert the exchange happened after operatorLoginCommand().Run returns.
var operatorOIDCExchanges = &atomic.Int64{}

// operatorOIDCExchangesCompleted reports whether at least one exchange completed
// during the current test's server lifetime.
func operatorOIDCExchangesCompleted() bool {
	return operatorOIDCExchanges.Load() > 0
}

// signTestIDToken signs an RS256 JWT with the given key and claims.
func signTestIDToken(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	header, err := json.Marshal(map[string]any{"alg": "RS256", "typ": "JWT"})
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
