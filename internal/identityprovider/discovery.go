package identityprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
)

type discoveryDocument struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
}

func FetchDiscovery(ctx context.Context, client *http.Client, issuer string) (application.IdentityDiscovery, error) {
	return FetchDiscoveryAt(ctx, client, issuer, issuer)
}

func FetchDiscoveryAt(ctx context.Context, client *http.Client, endpointIssuer, expectedIssuer string) (application.IdentityDiscovery, error) {
	endpointIssuer = strings.TrimRight(strings.TrimSpace(endpointIssuer), "/")
	expectedIssuer = strings.TrimRight(strings.TrimSpace(expectedIssuer), "/")
	if endpointIssuer == "" || expectedIssuer == "" {
		return application.IdentityDiscovery{}, fmt.Errorf("OIDC issuer is required")
	}
	for label, value := range map[string]string{"endpoint issuer": endpointIssuer, "expected issuer": expectedIssuer} {
		u, err := url.Parse(value)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
			return application.IdentityDiscovery{}, fmt.Errorf("OIDC %s must be an HTTPS URL without query or fragment", label)
		}
	}
	if client == nil {
		client = http.DefaultClient
	}
	body, err := fetchDiscoveryDocument(ctx, client, endpointIssuer+"/.well-known/openid-configuration")
	if err != nil {
		return application.IdentityDiscovery{}, err
	}
	var doc discoveryDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return application.IdentityDiscovery{}, fmt.Errorf("decode OIDC discovery: %w", err)
	}
	if strings.TrimRight(strings.TrimSpace(doc.Issuer), "/") != expectedIssuer {
		return application.IdentityDiscovery{}, fmt.Errorf("OIDC discovery issuer %q does not match configured issuer %q", doc.Issuer, expectedIssuer)
	}
	result := application.IdentityDiscovery{
		Issuer:                expectedIssuer,
		AuthorizationEndpoint: strings.TrimSpace(doc.AuthorizationEndpoint),
		TokenEndpoint:         strings.TrimSpace(doc.TokenEndpoint),
		UserinfoEndpoint:      strings.TrimSpace(doc.UserinfoEndpoint),
		JWKSURI:               strings.TrimSpace(doc.JWKSURI),
		EndSessionEndpoint:    strings.TrimSpace(doc.EndSessionEndpoint),
	}
	for label, value := range map[string]string{
		"authorization endpoint": result.AuthorizationEndpoint,
		"token endpoint":         result.TokenEndpoint,
		"userinfo endpoint":      result.UserinfoEndpoint,
		"jwks uri":               result.JWKSURI,
	} {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return application.IdentityDiscovery{}, fmt.Errorf("OIDC discovery %s is not an HTTPS URL", label)
		}
	}
	return result, nil
}

func fetchDiscoveryDocument(ctx context.Context, client *http.Client, endpoint string) ([]byte, error) {
	retryCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		req, err := http.NewRequestWithContext(retryCtx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err == nil {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close()
			if readErr != nil {
				return nil, readErr
			}
			if resp.StatusCode == http.StatusOK {
				return body, nil
			}
			lastErr = fmt.Errorf("discover OIDC provider: HTTP %d", resp.StatusCode)
			if resp.StatusCode != http.StatusInternalServerError && resp.StatusCode != http.StatusBadGateway &&
				resp.StatusCode != http.StatusServiceUnavailable && resp.StatusCode != http.StatusGatewayTimeout {
				return nil, lastErr
			}
		} else {
			lastErr = fmt.Errorf("discover OIDC provider: %w", err)
		}

		select {
		case <-retryCtx.Done():
			if ctx.Err() != nil {
				return nil, fmt.Errorf("discover OIDC provider: %w", ctx.Err())
			}
			return nil, lastErr
		case <-ticker.C:
		}
	}
}
