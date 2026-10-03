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
	"time"
)

type keycloakAdmin struct {
	endpoint string
	client   *http.Client
	user     string
	password string
	token    string
}

type keycloakAdminLoginError struct {
	Status int
	Body   string
}

func (e *keycloakAdminLoginError) Error() string {
	return fmt.Sprintf("authenticate Keycloak admin: HTTP %d: %s", e.Status, e.Body)
}

type keycloakRealm struct {
	ID                                            string            `json:"id,omitempty"`
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

type keycloakUserCredential struct {
	Type      string `json:"type"`
	Value     string `json:"value"`
	Temporary bool   `json:"temporary"`
}

type keycloakUser struct {
	ID            string                   `json:"id,omitempty"`
	Username      string                   `json:"username"`
	Email         string                   `json:"email,omitempty"`
	Enabled       bool                     `json:"enabled"`
	EmailVerified bool                     `json:"emailVerified,omitempty"`
	Credentials   []keycloakUserCredential `json:"credentials,omitempty"`
	Attributes    map[string][]string      `json:"attributes,omitempty"`
}

type keycloakRole struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
		return &keycloakAdminLoginError{
			Status: resp.StatusCode,
			Body:   strings.TrimSpace(string(body)),
		}
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

func (a *keycloakAdmin) reconcileUser(ctx context.Context, realm, username, password string, realmAdmin bool) error {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return fmt.Errorf("Keycloak development user credentials are incomplete")
	}
	query := url.Values{}
	query.Set("username", username)
	query.Set("exact", "true")
	path := "/admin/realms/" + url.PathEscape(realm) + "/users?" + query.Encode()
	status, body, err := a.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("inspect Keycloak user %s: HTTP %d: %s", username, status, body)
	}
	var users []keycloakUser
	if err := json.Unmarshal([]byte(body), &users); err != nil {
		return fmt.Errorf("decode Keycloak user lookup: %w", err)
	}
	if len(users) > 1 {
		return fmt.Errorf("Keycloak user %q is ambiguous", username)
	}

	desired := keycloakUser{
		Username:      username,
		Email:         username + "@baseharbor.local",
		Enabled:       true,
		EmailVerified: true,
		Credentials:   []keycloakUserCredential{{Type: "password", Value: password, Temporary: false}},
		Attributes: map[string][]string{
			"baseharbor.scope": []string{"developer-access"},
		},
	}

	userID := ""
	if len(users) == 0 {
		status, body, err = a.do(ctx, http.MethodPost, "/admin/realms/"+url.PathEscape(realm)+"/users", desired)
		if err != nil {
			return err
		}
		if status != http.StatusCreated {
			return fmt.Errorf("create Keycloak development user: HTTP %d: %s", status, body)
		}
		status, body, err = a.do(ctx, http.MethodGet, path, nil)
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return fmt.Errorf("resolve Keycloak development user: HTTP %d: %s", status, body)
		}
		users = nil
		if err := json.Unmarshal([]byte(body), &users); err != nil || len(users) != 1 {
			return fmt.Errorf("resolve Keycloak development user")
		}
		userID = users[0].ID
	} else {
		userID = users[0].ID
		desired.ID = userID
		status, body, err = a.do(ctx, http.MethodPut, "/admin/realms/"+url.PathEscape(realm)+"/users/"+url.PathEscape(userID), desired)
		if err != nil {
			return err
		}
		if status != http.StatusNoContent {
			return fmt.Errorf("update Keycloak development user: HTTP %d: %s", status, body)
		}
	}

	credential := keycloakUserCredential{Type: "password", Value: password, Temporary: false}
	status, body, err = a.do(ctx, http.MethodPut, "/admin/realms/"+url.PathEscape(realm)+"/users/"+url.PathEscape(userID)+"/reset-password", credential)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("reset Keycloak development user password: HTTP %d: %s", status, body)
	}

	if !realmAdmin {
		return nil
	}
	return a.ensureRealmAdminRole(ctx, realm, userID)
}

func (a *keycloakAdmin) ensureRealmAdminRole(ctx context.Context, realm, userID string) error {
	query := url.Values{}
	query.Set("clientId", "realm-management")
	lookupCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var clientID string
	var lastBody string
	for {
		status, body, err := a.do(lookupCtx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm)+"/clients?"+query.Encode(), nil)
		if err == nil && status == http.StatusOK {
			var clients []keycloakClient
			if json.Unmarshal([]byte(body), &clients) == nil && len(clients) == 1 && strings.TrimSpace(clients[0].ID) != "" {
				clientID = clients[0].ID
				break
			}
		}
		lastBody = body
		select {
		case <-lookupCtx.Done():
			return fmt.Errorf("resolve Keycloak realm-management client after HA convergence: %s", strings.TrimSpace(lastBody))
		case <-ticker.C:
		}
	}
	status, body, err := a.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm)+"/clients/"+url.PathEscape(clientID)+"/roles/realm-admin", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("resolve Keycloak realm-admin role: HTTP %d: %s", status, body)
	}
	var role keycloakRole
	if err := json.Unmarshal([]byte(body), &role); err != nil {
		return err
	}
	status, body, err = a.do(ctx, http.MethodPost, "/admin/realms/"+url.PathEscape(realm)+"/users/"+url.PathEscape(userID)+"/role-mappings/clients/"+url.PathEscape(clientID), []keycloakRole{role})
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusConflict {
		return fmt.Errorf("grant Keycloak realm-admin role: HTTP %d: %s", status, body)
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
	if err := json.Unmarshal([]byte(body), &currentRealm); err != nil {
		return err
	}
	if !currentRealm.Enabled || !keycloakRealmOwnedBy(currentRealm, ownership) {
		return fmt.Errorf("Keycloak realm ownership/readiness verification failed")
	}

	query := url.Values{}
	query.Set("clientId", clientID)
	status, body, err = a.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm)+"/clients?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("verify Keycloak client: HTTP %d: %s", status, body)
	}
	var clients []keycloakClient
	if err := json.Unmarshal([]byte(body), &clients); err != nil {
		return err
	}
	if len(clients) != 1 || !clients[0].Enabled || clients[0].PublicClient {
		return fmt.Errorf("Keycloak client readiness verification failed")
	}
	if !sameSortedStrings(clients[0].RedirectURIs, redirects) {
		return fmt.Errorf("Keycloak redirect URI drift detected")
	}
	wantLogouts := sortedUnique(logouts)
	gotLogouts := []string{}
	if raw := strings.TrimSpace(clients[0].Attributes["post.logout.redirect.uris"]); raw != "" {
		gotLogouts = sortedUnique(strings.Split(raw, "##"))
	}
	if !sameSortedStrings(gotLogouts, wantLogouts) {
		return fmt.Errorf("Keycloak logout URI drift detected")
	}

	if strings.EqualFold(mfa, "required") || passwordless {
		status, body, err = a.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm)+"/authentication/required-actions", nil)
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return fmt.Errorf("verify Keycloak required actions: HTTP %d: %s", status, body)
		}
		var actions []requiredAction
		if err := json.Unmarshal([]byte(body), &actions); err != nil {
			return err
		}
		byAlias := map[string]requiredAction{}
		for _, action := range actions {
			byAlias[action.Alias] = action
		}
		var required []string
		if strings.EqualFold(mfa, "required") {
			for _, method := range methods {
				switch method {
				case "totp":
					required = append(required, "CONFIGURE_TOTP")
				case "webauthn", "passkey":
					required = append(required, "webauthn-register")
				}
			}
		}
		if passwordless {
			required = append(required, "webauthn-register-passwordless")
		}
		for _, alias := range sortedUnique(required) {
			action, ok := byAlias[alias]
			if !ok || !action.Enabled || !action.DefaultAction {
				return fmt.Errorf("Keycloak required action %s is not enforced", alias)
			}
		}
	}
	return nil
}

func sameSortedStrings(a, b []string) bool {
	a = sortedUnique(a)
	b = sortedUnique(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func standardOIDCClaim(claim string) bool {
	switch strings.TrimSpace(claim) {
	case "sub", "iss", "aud", "exp", "iat", "auth_time", "nonce", "acr", "amr", "azp", "sid", "typ",
		"name", "given_name", "family_name", "middle_name", "nickname", "preferred_username", "profile",
		"picture", "website", "email", "email_verified", "gender", "birthdate", "zoneinfo", "locale",
		"phone_number", "phone_number_verified", "address", "updated_at":
		return true
	default:
		return false
	}
}

func (a *keycloakAdmin) reconcileRequiredActions(ctx context.Context, realm string, mfa string, methods []string, passwordless bool) error {
	status, body, err := a.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm)+"/authentication/required-actions", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("list Keycloak required actions: HTTP %d: %s", status, body)
	}
	var actions []requiredAction
	if err := json.Unmarshal([]byte(body), &actions); err != nil {
		return err
	}
	byAlias := map[string]requiredAction{}
	for _, action := range actions {
		byAlias[action.Alias] = action
	}

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
		if err != nil {
			return err
		}
		if status != http.StatusNoContent {
			return fmt.Errorf("configure Keycloak required action %s: HTTP %d: %s", alias, status, body)
		}
	}
	return nil
}

func (a *keycloakAdmin) deleteRealm(ctx context.Context, realm string, ownership map[string]string) error {
	path := "/admin/realms/" + url.PathEscape(realm)
	status, body, err := a.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	if status != http.StatusOK {
		return fmt.Errorf("inspect Keycloak realm before delete: HTTP %d: %s", status, body)
	}
	var current keycloakRealm
	if err := json.Unmarshal([]byte(body), &current); err != nil {
		return err
	}
	if !keycloakRealmOwnedBy(current, ownership) {
		return fmt.Errorf("Keycloak realm %q is not owned by this BaseHarbor application/environment; refusing delete", realm)
	}
	status, body, err = a.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound || status == http.StatusNoContent {
		return nil
	}
	return fmt.Errorf("delete Keycloak realm: HTTP %d: %s", status, body)
}

func keycloakRealmOwnedBy(current keycloakRealm, expected map[string]string) bool {
	if len(expected) == 0 || len(current.Attributes) == 0 {
		return false
	}
	for key, value := range expected {
		if current.Attributes[key] != value {
			return false
		}
	}
	return true
}

func (a *keycloakAdmin) do(ctx context.Context, method, path string, payload any) (int, string, error) {
	var payloadData []byte
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return 0, "", err
		}
		payloadData = data
	}

	attempt := func() (int, string, error) {
		var body io.Reader
		if payloadData != nil {
			body = bytes.NewReader(payloadData)
		}
		req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(a.endpoint, "/")+path, body)
		if err != nil {
			return 0, "", err
		}
		if payloadData != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if a.token != "" {
			req.Header.Set("Authorization", "Bearer "+a.token)
		}
		resp, err := a.client.Do(req)
		if err != nil {
			return 0, "", err
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		if err != nil {
			return 0, "", err
		}
		return resp.StatusCode, strings.TrimSpace(string(data)), nil
	}

	if method != http.MethodGet {
		return attempt()
	}

	retryCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastStatus int
	var lastBody string
	var lastErr error
	for {
		status, body, err := attempt()
		if err == nil && status != http.StatusServiceUnavailable {
			return status, body, nil
		}
		lastStatus, lastBody, lastErr = status, body, err
		select {
		case <-retryCtx.Done():
			if lastErr != nil {
				return 0, "", lastErr
			}
			return lastStatus, lastBody, nil
		case <-ticker.C:
		}
	}
}

func sortedUnique(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
