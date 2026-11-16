package externalprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
		return Verification{}, errors.New("external PostgreSQL connection or authentication failed")
	}
	defer conn.Close(context.Background())
	var one int
	if err := conn.QueryRow(verifyCtx, "SELECT 1").Scan(&one); err != nil {
		return Verification{}, errors.New("external PostgreSQL semantic query failed")
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
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{TLSClientConfig: tlsConfig},
	}
	discoveryURL := strings.TrimRight(reg.Endpoint, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return Verification{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Verification{}, errors.New("external OIDC discovery connection or TLS verification failed")
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMaterialBytes+1))
	if err != nil || len(body) > maxMaterialBytes || json.Unmarshal(body, &doc) != nil {
		return Verification{}, errors.New("external OIDC discovery document is invalid or exceeds its size limit")
	}
	if strings.TrimRight(doc.Issuer, "/") != strings.TrimRight(reg.Endpoint, "/") || !safeDiscoveryEndpoint(doc.AuthorizationEndpoint) || !safeDiscoveryEndpoint(doc.TokenEndpoint) || !safeDiscoveryEndpoint(doc.JWKSURI) {
		return Verification{}, fmt.Errorf("external OIDC discovery issuer or required HTTPS endpoints are invalid")
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

func safeDiscoveryEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return false
	}
	values, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return false
	}
	for key := range values {
		switch strings.NewReplacer("-", "", "_", "", ".", "").Replace(strings.ToLower(key)) {
		case "token", "accesstoken", "refreshtoken", "idtoken", "clientsecret", "password", "secret", "apikey", "authorization", "privatekey":
			return false
		}
	}
	return true
}
