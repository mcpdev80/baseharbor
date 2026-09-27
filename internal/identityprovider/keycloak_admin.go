package identityprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

type keycloakAdmin struct {
	endpoint string
	client   *http.Client
	user     string
	password string
	token    string
}

type keycloakRealm struct {
	Realm                                         string            `json:"realm"`
	Enabled                                       bool              `json:"enabled"`
	DisplayName                                   string            `json:"displayName,omitempty"`
	SSLRequired                                   string            `json:"sslRequired,omitempty"`
	BruteForceProtected                           bool              `json:"bruteForceProtected"`
	RegistrationAllowed                           bool              `json:"registrationAllowed"`
	ResetPasswordAllowed                          bool              `json:"resetPasswordAllowed"`
	RememberMe                                    bool              `json:"rememberMe"`
	Attributes                                    map[string]string `json:"attributes,omitempty"`
	OTPPolicyType                                 string            `json:"otpPolicyType,omitempty"`
	WebAuthnPolicyRpEntityName                    string            `json:"webAuthnPolicyRpEntityName,omitempty"`
	WebAuthnPolicySignatureAlgorithms             []string          `json:"webAuthnPolicySignatureAlgorithms,omitempty"`
	WebAuthnPolicyPasswordlessRpEntityName        string            `json:"webAuthnPolicyPasswordlessRpEntityName,omitempty"`
	WebAuthnPolicyPasswordlessSignatureAlgorithms []string          `json:"webAuthnPolicyPasswordlessSignatureAlgorithms,omitempty"`
}

type keycloakClient struct {
	ID                        string            `json:"id,omitempty"`
	ClientID                  string            `json:"clientId"`
	Name                      string            `json:"name,omitempty"`
	Enabled                   bool              `json:"enabled"`
	Protocol                  string            `json:"protocol"`
	PublicClient              bool              `json:"publicClient"`
	StandardFlowEnabled       bool              `json:"standardFlowEnabled"`
	DirectAccessGrantsEnabled bool              `json:"directAccessGrantsEnabled"`
	ServiceAccountsEnabled    bool              `json:"serviceAccountsEnabled"`
	Secret                    string            `json:"secret,omitempty"`
	RedirectURIs              []string          `json:"redirectUris,omitempty"`
	WebOrigins                []string          `json:"webOrigins,omitempty"`
	Attributes                map[string]string `json:"attributes,omitempty"`
	DefaultClientScopes       []string          `json:"defaultClientScopes,omitempty"`
	OptionalClientScopes      []string          `json:"optionalClientScopes,omitempty"`
}

type keycloakClientScope struct {
	ID         string            `json:"id,omitempty"`
	Name       string            `json:"name"`
	Protocol   string            `json:"protocol"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type keycloakProtocolMapper struct {
	ID             string            `json:"id,omitempty"`
	Name           string            `json:"name"`
	Protocol       string            `json:"protocol"`
	ProtocolMapper string            `json:"protocolMapper"`
	Config         map[string]string `json:"config"`
}

type requiredAction struct {
	Alias         string `json:"alias"`
	Name          string `json:"name,omitempty"`
	ProviderID    string `json:"providerId,omitempty"`
	Enabled       bool   `json:"enabled"`
	DefaultAction bool   `json:"defaultAction"`
	Priority      int    `json:"priority,omitempty"`
}

func (a *keycloakAdmin) login(ctx context.Context) error {
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", "admin-cli")
	form.Set("username", a.user)
	form.Set("password", a.password)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.endpoint, "/")+"/realms/master/protocol/openid-connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("authenticate Keycloak admin: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("authenticate Keycloak admin: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return err
	}
	a.token = strings.TrimSpace(payload.AccessToken)
	if a.token == "" {
		return fmt.Errorf("Keycloak admin token is empty")
	}
	return nil
}

func (a *keycloakAdmin) reconcileRealm(ctx context.Context, desired keycloakRealm) error {
	path := "/admin/realms/" + url.PathEscape(desired.Realm)
	status, body, err := a.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	switch status {
	case http.StatusOK:
		var current keycloakRealm
		if err := json.Unmarshal([]byte(body), &current); err != nil {
			return fmt.Errorf("decode existing Keycloak realm: %w", err)
		}
		if !keycloakRealmOwnedBy(current, desired.Attributes) {
			return fmt.Errorf("Keycloak realm %q exists but is not owned by this BaseHarbor application/environment", desired.Realm)
		}
		status, body, err = a.do(ctx, http.MethodPut, path, desired)
		if err != nil {
			return err
		}
		if status != http.StatusNoContent {
			return fmt.Errorf("update Keycloak realm: HTTP %d: %s", status, body)
		}
	case http.StatusNotFound:
		status, body, err = a.do(ctx, http.MethodPost, "/admin/realms", desired)
		if err != nil {
			return err
		}
		if status != http.StatusCreated {
			return fmt.Errorf("create Keycloak realm: HTTP %d: %s", status, body)
		}
	default:
		return fmt.Errorf("inspect Keycloak realm: HTTP %d", status)
	}
	return nil
}

func (a *keycloakAdmin) reconcileClient(ctx context.Context, realm string, desired keycloakClient) (string, error) {
	query := url.Values{}
	query.Set("clientId", desired.ClientID)
	status, body, err := a.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm)+"/clients?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("inspect Keycloak client: HTTP %d: %s", status, body)
	}
	var existing []keycloakClient
	if err := json.Unmarshal([]byte(body), &existing); err != nil {
		return "", fmt.Errorf("decode Keycloak client lookup: %w", err)
	}
	if len(existing) > 1 {
		return "", fmt.Errorf("Keycloak client %q is ambiguous", desired.ClientID)
	}
	if len(existing) == 0 {
		status, body, err = a.do(ctx, http.MethodPost, "/admin/realms/"+url.PathEscape(realm)+"/clients", desired)
		if err != nil {
			return "", err
		}
		if status != http.StatusCreated {
			return "", fmt.Errorf("create Keycloak client: HTTP %d: %s", status, body)
		}
		status, body, err = a.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm)+"/clients?"+query.Encode(), nil)
		if err != nil {
			return "", err
		}
		if status != http.StatusOK {
			return "", fmt.Errorf("resolve created Keycloak client: HTTP %d", status)
		}
		existing = nil
		if err := json.Unmarshal([]byte(body), &existing); err != nil || len(existing) != 1 {
			return "", fmt.Errorf("resolve created Keycloak client")
		}
		return existing[0].ID, nil
	}
	desired.ID = existing[0].ID
	status, body, err = a.do(ctx, http.MethodPut, "/admin/realms/"+url.PathEscape(realm)+"/clients/"+url.PathEscape(existing[0].ID), desired)
	if err != nil {
		return "", err
	}
	if status != http.StatusNoContent {
		return "", fmt.Errorf("update Keycloak client: HTTP %d: %s", status, body)
	}
	return existing[0].ID, nil
}

func (a *keycloakAdmin) reconcileClientScopes(ctx context.Context, realm, clientUUID string, scopes, claims []string) error {
	status, body, err := a.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm)+"/client-scopes", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("list Keycloak client scopes: HTTP %d: %s", status, body)
	}
	var available []keycloakClientScope
	if err := json.Unmarshal([]byte(body), &available); err != nil {
		return err
	}
	byName := map[string]keycloakClientScope{}
	for _, scope := range available {
		byName[scope.Name] = scope
	}

	for _, name := range sortedUnique(scopes) {
		if name == "openid" {
			continue
		}
		scope, ok := byName[name]
		if !ok {
			desired := keycloakClientScope{
				Name: name, Protocol: "openid-connect",
				Attributes: map[string]string{
					"include.in.token.scope":    "true",
					"display.on.consent.screen": "true",
				},
			}
			status, body, err := a.do(ctx, http.MethodPost, "/admin/realms/"+url.PathEscape(realm)+"/client-scopes", desired)
			if err != nil {
				return err
			}
			if status != http.StatusCreated {
				return fmt.Errorf("create Keycloak client scope %s: HTTP %d: %s", name, status, body)
			}
			status, body, err = a.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm)+"/client-scopes", nil)
			if err != nil {
				return err
			}
			if status != http.StatusOK {
				return fmt.Errorf("reload Keycloak client scopes: HTTP %d", status)
			}
			available = nil
			if err := json.Unmarshal([]byte(body), &available); err != nil {
				return err
			}
			byName = map[string]keycloakClientScope{}
			for _, item := range available {
				byName[item.Name] = item
			}
			scope, ok = byName[name]
			if !ok {
				return fmt.Errorf("created Keycloak client scope %q cannot be resolved", name)
			}
		}
		status, body, err := a.do(ctx, http.MethodPut, "/admin/realms/"+url.PathEscape(realm)+"/clients/"+url.PathEscape(clientUUID)+"/default-client-scopes/"+url.PathEscape(scope.ID), nil)
		if err != nil {
			return err
		}
		if status != http.StatusNoContent && status != http.StatusConflict {
			return fmt.Errorf("attach Keycloak client scope %s: HTTP %d: %s", name, status, body)
		}
	}

	status, body, err = a.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm)+"/clients/"+url.PathEscape(clientUUID)+"/protocol-mappers/models", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("list Keycloak protocol mappers: HTTP %d: %s", status, body)
	}
	var mappers []keycloakProtocolMapper
	if err := json.Unmarshal([]byte(body), &mappers); err != nil {
		return err
	}
	mapperByName := map[string]keycloakProtocolMapper{}
	for _, mapper := range mappers {
		mapperByName[mapper.Name] = mapper
	}
	for _, claim := range sortedUnique(claims) {
		if standardOIDCClaim(claim) {
			continue
		}
		name := "baseharbor-claim-" + claim
		desired := keycloakProtocolMapper{
			Name: name, Protocol: "openid-connect", ProtocolMapper: "oidc-usermodel-attribute-mapper",
			Config: map[string]string{
				"user.attribute":       claim,
				"claim.name":           claim,
				"jsonType.label":       "String",
				"id.token.claim":       "true",
				"access.token.claim":   "true",
				"userinfo.token.claim": "true",
			},
		}
		if existing, ok := mapperByName[name]; ok {
			desired.ID = existing.ID
			status, body, err = a.do(ctx, http.MethodPut, "/admin/realms/"+url.PathEscape(realm)+"/clients/"+url.PathEscape(clientUUID)+"/protocol-mappers/models/"+url.PathEscape(existing.ID), desired)
		} else {
			status, body, err = a.do(ctx, http.MethodPost, "/admin/realms/"+url.PathEscape(realm)+"/clients/"+url.PathEscape(clientUUID)+"/protocol-mappers/models", desired)
		}
		if err != nil {
			return err
		}
		if status != http.StatusNoContent && status != http.StatusCreated {
			return fmt.Errorf("reconcile Keycloak claim mapper %s: HTTP %d: %s", claim, status, body)
		}
	}
	return nil
}

func (a *keycloakAdmin) verifyManagedIdentity(ctx context.Context, realm string, ownership map[string]string, clientID string, redirects, logouts []string, mfa string, methods []string, passwordless bool) error {
	status, body, err := a.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm), nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("verify Keycloak realm: HTTP %d: %s", status, body)
	}
	var currentRealm keycloakRealm
	if err := j