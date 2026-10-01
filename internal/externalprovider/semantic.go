package externalprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func verifySemantic(ctx context.Context, reg Registration) (Verification, bool, error) {
	if reg.Provider.Supports(capability.SQL) {
		result, err := verifyPostgreSQL(ctx, reg)
		return result, true, err
	}
	if reg.Provider.Supports(capability.Identity) {
		result, err := verifyOIDC(ctx, reg)
		return result, true, err
	}
	return Verification{}, false, nil
}

func verifyPostgreSQL(ctx context.Context, reg Registration) (Verification, error) {
	u, err := url.Parse(reg.Endpoint)
	if err != nil {
		return Verification{}, err
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return Verification{}, fmt.Errorf("database.sql external provider requires postgres:// or postgresql:// endpoint")
	}
	creds, err := ResolveCredentialReference(reg.CredentialRef)
	if err != nil {
		return Verification{}, err
	}
	config, err := pgx.ParseConfig(reg.Endpoint)
	if err != nil {
		return Verification{}, fmt.Errorf("parse PostgreSQL endpoint: %w", err)
	}
	if creds.Username != "" {
		config.User = creds.Username
	}
	if creds.Password != "" {
		config.Password = creds.Password
	}
	if config.User == "" {
		return Verification{}, fmt.Errorf("PostgreSQL verification requires username through credential_ref")
	}
	tlsConfig, err := tlsConfigForRegistration(reg, u.Hostname())
	if err != nil {
		return Verification{}, err
	}
	config.TLSConfig = tlsConfig
	config.ConnectTimeout = 10 * time.Second
	verifyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, err := pgx.ConnectConfig(verifyCtx, config)
	if err != nil {
		return Verification{}, fmt.Errorf("verify external PostgreSQL binding: %w", err)
	}
	defer conn.Close(context.Background())
	var one int
	if err := conn.QueryRow(verifyCtx, "SELECT 1").Scan(&one); err != nil {
		return Verification{}, fmt.Errorf("verify external PostgreSQL semantic path: %w", err)
	}
	if one != 1 {
		return Verification{}, fmt.Errorf("external PostgreSQL semantic verification returned %d", one)
	}
	return Verification{
		ID: reg.ID, Endpoint: reg.Endpoint, Reachability: "ok", TLS: "ok",
		Semantic: "ok", Capability: string(capability.SQL),
		Detail: "PostgreSQL application-facing SELECT 1 succeeded with configured credential/trust references",
	}, nil
}

func verifyOIDC(ctx context.Context, reg Registration) (Verification, error) {
	u, err := url.Parse(reg.Endpoint)
	if err != nil {
		return Verification{}, err
	}
	if u.Scheme != "https" {
		return Verification{}, fmt.Errorf("identity.oidc external provider requires an https issuer endpoint")
	}
	tlsConfig, err := tlsConfigForRegistration(reg, u.Hostname())
	if err != nil {
		return Verification{}, err
	}
	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
	}
	discoveryURL := strings.TrimRight(reg.Endpoint, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return Verification{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Verification{}, fmt.Errorf("verify external OIDC discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Verification{}, fmt.Errorf("external OIDC discovery returned HTTP %d", resp.StatusCode)
	}
	var doc struct {
		Issuer                string `json:"issuer"`
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
		JWKSURI               string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return Verification{}, fmt.Errorf("decode external OIDC discovery: %w", err)
	}
	if strings.TrimSpace(doc.Issuer) == "" || strings.TrimSpace(doc.AuthorizationEndpoint) == "" || strings.TrimSpace(doc.JWKSURI) == "" {
		return Verification{}, fmt.Errorf("external OIDC discovery is missing issuer, authorization_endpoint or jwks_uri")
	}
	return Verification{
		ID: reg.ID, Endpoint: reg.Endpoint, Reachability: "ok", TLS: "ok",
		Semantic: "ok", Capability: string(capability.Identity),
		Detail: "OIDC discovery succeeded through configured TLS trust",
	}, nil
}

func tlsStatusForScheme(scheme string) string {
	switch strings.ToLower(strings.TrimSpace(scheme)) {
	case "https", "tls", "rediss", "amqps", "ldaps", "postgres", "postgresql":
		return "ok"
	default:
		return "not_applicable"
	}
}
