package identityprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

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
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	if issuer == "" {
		return application.IdentityDiscovery{}, fmt.Errorf("OIDC issuer is required")
	}
	u, err := url.Parse(issuer)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return application.IdentityDiscovery{}, fmt.Errorf("OIDC issuer must be an HTTPS URL without query or fragment")
	}
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return application.IdentityDiscovery{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return application.IdentityDiscovery{}, fmt.Errorf("discover OIDC provider: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return application.IdentityDiscovery{}, fmt.Errorf("discover OIDC provider: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return application.IdentityDiscovery{}, err
	}
	var doc discoveryDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return application.IdentityDiscovery{}, fmt.Errorf("decode OIDC discovery: %w", err)
	}
	if strings.TrimRight(strings.TrimSpace(doc.Issuer), "/") != issuer {
		return application.IdentityDiscovery{}, fmt.Errorf("OIDC discovery issuer %q does not match configured issuer %q", doc.Issuer, issuer)
	}
	result := application.IdentityDiscovery{
		Issuer:                issuer,
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
