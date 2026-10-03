package identityprovider

import (
	"context"
	"errors"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

var errKeycloakClientSecretRejected = errors.New("Keycloak rejected client secret")

type keycloakSecretCredential struct {
	Value string `json:"value"`
}

func (a *keycloakAdmin) currentClientSecret(ctx context.Context, realm, clientUUID string) (string, error) {
	status, body, err := a.do(ctx, http.MethodGet, keycloakAdminClientSecretPath(realm, clientUUID), nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("read Keycloak client secret: HTTP %d: %s", status, body)
	}
	var credential keycloakSecretCredential
	if err := json.Unmarshal([]byte(body), &credential); err != nil {
		return "", fmt.Errorf("decode Keycloak client secret: %w", err)
	}
	credential.Value = strings.TrimSpace(credential.Value)
	if credential.Value == "" {
		return "", fmt.Errorf("Keycloak client secret is empty")
	}
	return credential.Value, nil
}

func (a *keycloakAdmin) rotateClientSecret(ctx context.Context, realm, clientUUID string) (current, rotated string, err error) {
	status, body, err := a.do(ctx, http.MethodPost, keycloakAdminClientSecretPath(realm, clientUUID), nil)
	if err != nil {
		return "", "", err
	}
	if status != http.StatusOK {
		return "", "", fmt.Errorf("rotate Keycloak client secret: HTTP %d: %s", status, body)
	}
	var currentCredential keycloakSecretCredential
	if err := json.Unmarshal([]byte(body), &currentCredential); err != nil {
		return "", "", fmt.Errorf("decode rotated Keycloak client secret: %w", err)
	}
	current = strings.TrimSpace(currentCredential.Value)
	if current == "" {
		return "", "", fmt.Errorf("rotated Keycloak client secret is empty")
	}

	status, body, err = a.do(ctx, http.MethodGet, keycloakAdminClientSecretPath(realm, clientUUID)+"/rotated", nil)
	if err != nil {
		return "", "", err
	}
	if status != http.StatusOK {
		return "", "", fmt.Errorf("read Keycloak overlap secret: HTTP %d: %s", status, body)
	}
	var rotatedCredential keycloakSecretCredential
	if err := json.Unmarshal([]byte(body), &rotatedCredential); err != nil {
		return "", "", fmt.Errorf("decode Keycloak overlap secret: %w", err)
	}
	rotated = strings.TrimSpace(rotatedCredential.Value)
	if rotated == "" || rotated == current {
		return "", "", fmt.Errorf("Keycloak did not preserve a distinct rotated client secret")
	}
	return current, rotated, nil
}

func (a *keycloakAdmin) retireRotatedClientSecret(ctx context.Context, realm, clientUUID string) error {
	status, body, err := a.do(ctx, http.MethodDelete, keycloakAdminClientSecretPath(realm, clientUUID)+"/rotated", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return fmt.Errorf("retire Keycloak rotated client secret: HTTP %d: %s", status, body)
	}
	return nil
}

func (a *keycloakAdmin) verifyClientSecretAuthentication(ctx context.Context, realm, clientID, secret string) error {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", clientID)
	form.Set("client_secret", secret)
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(a.endpoint, "/")+"/realms/"+url.PathEscape(realm)+"/protocol/openid-connect/token",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("verify Keycloak client secret: %w", err)
	}
	defer resp.Body.Close()
	var payload struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	if strings.EqualFold(strings.TrimSpace(payload.Error), "invalid_client") || resp.StatusCode == http.StatusUnauthorized {
		return errKeycloakClientSecretRejected
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("Keycloak client-secret verification failed: HTTP %d", resp.StatusCode)
	}
	// The managed application client intentionally has service accounts disabled.
	// A valid client secret can therefore return unauthorized_client/unsupported_grant
	// for client_credentials while still proving that client authentication passed.
	return nil
}

func keycloakAdminClientSecretPath(realm, clientUUID string) string {
	return "/admin/realms/" + url.PathEscape(realm) + "/clients/" + url.PathEscape(clientUUID) + "/client-secret"
}
