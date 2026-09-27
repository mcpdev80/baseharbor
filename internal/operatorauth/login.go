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
	Out    io.Writer
	ErrOut io.Writer
}

type interactiveKey struct{}

func WithInteractive(ctx context.Context, out, errOut io.Writer) context.Context {
	return context.WithValue(ctx, interactiveKey{}, Interactive{Out: out, ErrOut: errOut})
}

func interactiveFromContext(ctx context.Context) (Interactive, bool) {
	value, ok := ctx.Value(interactiveKey{}).(Interactive)
	return value, ok && value.Out != nil
}

func Login(ctx context.Context, cfg Config, out io.Writer) (*identity.Principal, error) {
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("discover operator OIDC provider: %w", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
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

	type callbackResult struct {
		code string
		err  error
	}
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

	if out != nil {
		fmt.Fprintf(out, "Opening operator login in your browser.\nIf it does not open, use:\n%s\n", authURL)
	}
	_ = openBrowser(authURL)

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
		Issuer:    principal.Issuer,
		Subject:   principal.Subject,
		ClientID:  cfg.ClientID,
		IDToken:   rawIDToken,
		ExpiresAt: expiry,
	}
	if err := SaveSession(session); err != nil {
		return nil, err
	}
	return principal, nil
}

func Ensure(ctx context.Context, environment string) (context.Context, error) {
	if !ManagedEnvironment(environment) {
		return ctx, nil
	}
	cfg, err := ConfigFromEnv()
	if err != nil {
		return ctx, err
	}
	principal, err := VerifySession(ctx, cfg)
	if err == nil {
		return identity.WithPrincipal(ctx, principal), nil
	}
	if !errors.Is(err, ErrAuthenticationRequired) {
		return ctx, err
	}

	interactive, ok := interactiveFromContext(ctx)
	if !ok {
		return ctx, ErrAuthenticationRequired
	}
	principal, err = Login(ctx, cfg, interactive.Out)
	if err != nil {
		return ctx, err
	}
	return identity.WithPrincipal(ctx, principal), nil
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
