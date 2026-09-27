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
	Realm        string   `json:"realm"`
	Enabled      bool     `json:"enabled"`
	DisplayName  string   `json:"displayName,omitempty"`
	SSLRequired  string   `json:"sslRequired,omitempty"`
	BruteForceProtected bool `json:"bruteForceProtected"`
	RegistrationAllowed bool `json:"registrationAllowed"`
	ResetPasswordAllowed bool `json:"resetPasswordAllowed"`
	RememberMe   bool     `json:"rememberMe"`
	OTPPolicyType string   `json:"otpPolicyType,omitempty"`
	WebAuthnPolicyRpEntityName string `json:"webAuthnPolicyRpEntityName,omitempty"`
	WebAuthnPolicySignatureAlgorithms []string `json:"webAuthnPolicySignatureAlgorithms,omitempty"`
	WebAuthnPolicyPasswordlessRpEntityName string `json:"webAuthnPolicyPasswordlessRpEntityName,omitempty"`
	WebAuthnPolicyPasswordlessSignatureAlgorithms []string `json:"webAuthnPolicyPasswordlessSignatureAlgorithms,omitempty"`
}

type keycloakClient struct {
	ID                    string            `json:"id,omitempty"`
	ClientID              string            `json:"clientId"`
	Name                  string            `json:"name,omitempty"`
	Enabled               bool              `json:"enabled"`
	Protocol              string            `json:"protocol"`
	PublicClient          bool              `json:"publicClient"`
	StandardFlowEnabled   bool              `json:"standardFlowEnabled"`
	DirectAccessGrantsEnabled bool           `json:"directAccessGrantsEnabled"`
	ServiceAccountsEnabled bool              `json:"serviceAccountsEnabled"`
	Secret                string            `json:"secret,omitempty"`
	RedirectURIs          []string          `json:"redirectUris,omitempty"`
	WebOrigins            []string          `json:"webOrigins,omitempty"`
	Attributes            map[string]string `json:"attributes,omitempty"`
	DefaultClientScopes   []string          `json:"defaultClientScopes,omitempty"`
	OptionalClientScopes  []string          `json:"optionalClientScopes,omitempty"`
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
	if err != nil { return err }
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil { return fmt.Errorf("authenticate Keycloak admin: %w", err) }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("authenticate Keycloak admin: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct{ AccessToken string `json:"access_token"` }
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil { return err }
	a.token = strings.TrimSpace(payload.AccessToken)
	if a.token == "" { return fmt.Errorf("Keycloak admin token is empty") }
	return nil
}

func (a *keycloakAdmin) reconcileRealm(ctx context.Context, desired keycloakRealm) error {
	path := "/admin/realms/" + url.PathEscape(desired.Realm)
	status, _, err := a.do(ctx, http.MethodGet, path, nil)
	if err != nil { return err }
	switch status {
	case http.StatusOK:
		status, body, err := a.do(ctx, http.MethodPut, path, desired)
		if err != nil { return err }
		if status != http.StatusNoContent {
			return fmt.Errorf("update Keycloak realm: HTTP %d: %s", status, body)
		}
	case http.StatusNotFound:
		status, body, err := a.do(ctx, http.MethodPost, "/admin/realms", desired)
		if err != nil { return err }
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
	if err != nil { return "", err }
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
		if err != nil { return "", err }
		if status != http.StatusCreated {
			return "", fmt.Errorf("create Keycloak client: HTTP %d: %s", status, body)
		}
		status, body, err = a.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm)+"/clients?"+query.Encode(), nil)
		if err != nil { return "", err }
		if status != http.StatusOK { return "", fmt.Errorf("resolve created Keycloak client: HTTP %d", status) }
		existing = nil
		if err := json.Unmarshal([]byte(body), &existing); err != nil || len(existing) != 1 {
			return "", fmt.Errorf("resolve created Keycloak client")
		}
		return existing[0].ID, nil
	}
	desired.ID = existing[0].ID
	status, body, err = a.do(ctx, http.MethodPut, "/admin/realms/"+url.PathEscape(realm)+"/clients/"+url.PathEscape(existing[0].ID), desired)
	if err != nil { return "", err }
	if status != http.StatusNoContent {
		return "", fmt.Errorf("update Keycloak client: HTTP %d: %s", status, body)
	}
	return existing[0].ID, nil
}

func (a *keycloakAdmin) reconcileRequiredActions(ctx context.Context, realm string, mfa string, methods []string, passwordless bool) error {
	status, body, err := a.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm)+"/authentication/required-actions", nil)
	if err != nil { return err }
	if status != http.StatusOK { return fmt.Errorf("list Keycloak required actions: HTTP %d: %s", status, body) }
	var actions []requiredAction
	if err := json.Unmarshal([]byte(body), &actions); err != nil { return err }
	byAlias := map[string]requiredAction{}
	for _, action := range actions { byAlias[action.Alias] = action }

	required := map[string]bool{}
	if strings.EqualFold(mfa, "required") {
		for _, method := range methods {
			switch method {
			case "totp":
				required["CONFIGURE_TOTP"] = true
			case "webauthn", "passkey":
				required["webauthn-register"] = true
			}
		}
	}
	if passwordless {
		required["webauthn-register-passwordless"] = true
	}
	for alias, desiredDefault := range required {
		action, ok := byAlias[alias]
		if !ok {
			return fmt.Errorf("Keycloak does not advertise required action %q", alias)
		}
		action.Enabled = true
		action.DefaultAction = desiredDefault
		status, body, err := a.do(ctx, http.MethodPut, "/admin/realms/"+url.PathEscape(realm)+"/authentication/required-actions/"+url.PathEscape(alias), action)
		if err != nil { return err }
		if status != http.StatusNoContent {
			return fmt.Errorf("configure Keycloak required action %s: HTTP %d: %s", alias, status, body)
		}
	}
	return nil
}

func (a *keycloakAdmin) deleteRealm(ctx context.Context, realm string) error {
	status, body, err := a.do(ctx, http.MethodDelete, "/admin/realms/"+url.PathEscape(realm), nil)
	if err != nil { return err }
	if status == http.StatusNotFound || status == http.StatusNoContent { return nil }
	return fmt.Errorf("delete Keycloak realm: HTTP %d: %s", status, body)
}

func (a *keycloakAdmin) do(ctx context.Context, method, path string, payload any) (int, string, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil { return 0, "", err }
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(a.endpoint, "/")+path, body)
	if err != nil { return 0, "", err }
	if payload != nil { req.Header.Set("Content-Type", "application/json") }
	if a.token != "" { req.Header.Set("Authorization", "Bearer "+a.token) }
	resp, err := a.client.Do(req)
	if err != nil { return 0, "", err }
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil { return 0, "", err }
	return resp.StatusCode, strings.TrimSpace(string(data)), nil
}

func sortedUnique(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" { continue }
		if _, ok := seen[value]; ok { continue }
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
