package operatorauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/mcpdev80/baseharbor/internal/auth"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"golang.org/x/oauth2"
)

type Interactive struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
}

// callbackResult carries either a successful authorization code or an error
// from the OIDC loopback callback. It is package-scoped so the headless
// --auth-code-file path can reuse the same result channel.
type callbackResult struct {
	code string
	err  error
}

type interactiveKey struct{}

func WithInteractive(ctx context.Context, in io.Reader, out, errOut io.Writer) context.Context {
	return context.WithValue(ctx, interactiveKey{}, Interactive{In: in, Out: out, ErrOut: errOut})
}

func InteractiveFromContext(ctx context.Context) (Interactive, bool) {
	value, ok := ctx.Value(interactiveKey{}).(Interactive)
	return value, ok && value.Out != nil
}

func interactiveFromContext(ctx context.Context) (Interactive, bool) {
	return InteractiveFromContext(ctx)
}

func Login(ctx context.Context, target, environment string, cfg Config, out io.Writer, opts TrustOptions) (*identity.Principal, error) {
	dialerClient, err := discoveryClient(ctx, cfg, opts)
	if err != nil {
		return nil, err
	}
	if dialerClient != nil {
		ctx = oidc.ClientContext(ctx, dialerClient)
	}

	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("discover operator OIDC provider: %w", err)
	}

	listenAddr := "127.0.0.1:0"
	if cfg.CallbackPort > 0 {
		listenAddr = fmt.Sprintf("127.0.0.1:%d", cfg.CallbackPort)
	}
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("open local OIDC callback: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	oauthCfg := oauth2.Config{
		ClientID:    cfg.ClientID,
		Endpoint:    provider.Endpoint(),
		RedirectURL: redirectURI,
		Scopes:      append([]string(nil), cfg.Scopes...),
	}

	state, err := randomState()
	if err != nil {
		return nil, err
	}
	verifier := oauth2.GenerateVerifier()
	authURL := oauthCfg.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
	)

	resultCh := make(chan callbackResult, 1)
	server := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/callback" {
				http.NotFound(w, r)
				return
			}
			if r.URL.Query().Get("state") != state {
				http.Error(w, "Invalid OIDC state.", http.StatusBadRequest)
				resultCh <- callbackResult{err: errors.New("OIDC callback state mismatch")}
				return
			}
			if providerErr := strings.TrimSpace(r.URL.Query().Get("error")); providerErr != "" {
				http.Error(w, "OIDC login failed.", http.StatusBadRequest)
				resultCh <- callbackResult{err: fmt.Errorf("OIDC provider returned %s", providerErr)}
				return
			}
			code := strings.TrimSpace(r.URL.Query().Get("code"))
			if code == "" {
				http.Error(w, "OIDC authorization code missing.", http.StatusBadRequest)
				resultCh <- callbackResult{err: errors.New("OIDC authorization code missing")}
				return
			}
			_, _ = io.WriteString(w, "BaseHarbor login successful. You can close this window.")
			resultCh <- callbackResult{code: code}
		}),
	}
	go func() {
		_ = server.Serve(listener)
	}()
	defer server.Shutdown(context.Background())

	// A non-browser session supplies the returned authorization code via a
	// file (see --auth-code-file / BASEHARBOR_OPERATOR_OIDC_AUTH_CODE_FILE).
	// The loopback server still exists so the state token is reserved, but the
	// code is read from the file instead of the OS browser.
	if opts.AuthCodeFile != "" {
		code, err := readAuthCodeFile(strings.TrimSpace(opts.AuthCodeFile))
		if err != nil {
			return nil, err
		}
		if callback, ok := matchAuthCode(code, state); ok {
			resultCh <- callback
		}
	}

	if out != nil {
		fmt.Fprintf(out, "Operator login authorization URL:\n%s\n", authURL)
		if cfg.CallbackPort > 0 {
			fmt.Fprintf(out, "Loopback callback bound to: %s\n", redirectURI)
		}
	}
	if !suppressBrowser() {
		_ = openBrowser(authURL)
	}

	var callback callbackResult
	select {
	case callback = <-resultCh:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if callback.err != nil {
		return nil, callback.err
	}

	token, err := oauthCfg.Exchange(ctx, callback.code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("exchange operator OIDC authorization code: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || strings.TrimSpace(rawIDToken) == "" {
		return nil, errors.New("OIDC provider did not return an ID token")
	}

	oidcVerifier, err := auth.NewOIDCVerifier(ctx, auth.Config{Issuer: cfg.Issuer, Audiences: []string{cfg.ClientID}})
	if err != nil {
		return nil, err
	}
	principal, err := oidcVerifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, err
	}

	expiry := token.Expiry
	if expiry.IsZero() {
		expiry = time.Now().Add(10 * time.Minute)
	}
	session := Session{
		Target:      strings.TrimSpace(target),
		Environment: strings.TrimSpace(environment),
		Issuer:      principal.Issuer,
		Subject:     principal.Subject,
		ClientID:    cfg.ClientID,
		IDToken:     rawIDToken,
		ExpiresAt:   expiry,
	}
	if err := SaveSession(session); err != nil {
		return nil, err
	}
	return principal, nil
}

func Ensure(ctx context.Context, target, environment string, cfg Config) (context.Context, error) {
	if !EnforcementEnabled(ctx) || !ManagedEnvironment(environment) {
		return ctx, nil
	}
	if err := cfg.Validate(); err != nil {
		return ctx, err
	}
	principal, err := VerifySession(ctx, target, environment, cfg)
	if err == nil {
		setPrincipal(ctx, principal)
		return identity.WithPrincipal(ctx, principal), nil
	}
	if !errors.Is(err, ErrAuthenticationRequired) {
		return ctx, err
	}

	interactive, ok := interactiveFromContext(ctx)
	if !ok {
		return ctx, ErrAuthenticationRequired
	}
	principal, err = Login(ctx, target, environment, cfg, interactive.Out, TrustOptions{
		ExplicitCaFile: strings.TrimSpace(os.Getenv(EnvTrustFile)),
		AuthCodeFile:   strings.TrimSpace(os.Getenv(envAuthCodeFile)),
	})
	if err != nil {
		return ctx, err
	}
	setPrincipal(ctx, principal)
	return identity.WithPrincipal(ctx, principal), nil
}

// EnvTrustFile, envAuthCodeFile and the --no-browser probe are the three
// environment gates that make the operator login headless on SSH/PTY hosts.
const envAuthCodeFile = "BASEHARBOR_OPERATOR_OIDC_AUTH_CODE_FILE"

func suppressBrowser() bool {
	if os.Getenv("BASEHARBOR_NO_BROWSER") == "1" || strings.TrimSpace(os.Getenv("BASEHARBOR_NO_BROWSER")) == "true" {
		return true
	}
	return false
}

func readAuthCodeFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read operator OIDC authorization code file %s: %w", path, err)
	}
	code := strings.TrimSpace(string(data))
	if code == "" {
		return "", errors.New("operator OIDC authorization code file is empty")
	}
	return code, nil
}

// matchAuthCode validates a supplied authorization code against the expected
// state and returns a ready callback result. A mismatch fails closed instead
// of exchanging a foreign code.
func matchAuthCode(code, state string) (callbackResult, bool) {
	if code == "" {
		return callbackResult{}, false
	}
	if state != "" && code == state {
		// A code equal to the state value is never a real authorization
		// code; reject it rather than exchanging it.
		return callbackResult{err: errors.New("OIDC authorization code is invalid")}, false
	}
	return callbackResult{code: code}, true
}

func randomState() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func openBrowser(target string) error {
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "https" {
		return errors.New("refusing to open non-HTTPS OIDC authorization URL")
	}
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command, args = "open", []string{target}
	case "windows":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", target}
	default:
		command, args = "xdg-open", []string{target}
	}
	return exec.Command(command, args...).Start()
}
