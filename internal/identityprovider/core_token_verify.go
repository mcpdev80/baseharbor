package identityprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// VerifyCoreOperatorTokenFlow performs a real OAuth password grant against
// the installation-owned Keycloak admin identity and requires an authenticated
// Admin REST response over provisioned TLS. Built-in admin-cli uses lightweight
// tokens, which Keycloak 26.7 rejects at UserInfo. The Core realm, ownership
// and OIDC discovery are verified independently before this token exercise.
func VerifyCoreOperatorTokenFlow(ctx context.Context, dataDir, namespace, installationID, issuer string) error {
	if err := VerifyCoreIdentity(ctx, dataDir, namespace, installationID, issuer); err != nil {
		return err
	}
	files, err := ExistingCoreRuntimeFiles(dataDir, namespace)
	if err != nil {
		return err
	}
	admin, err := operatorKeycloakAdmin(ctx, files)
	if err != nil {
		return errors.New("Keycloak protected operator OAuth token issuance failed")
	}
	if err := verifyCoreAdminToken(ctx, admin.client, files.AdminURL, admin.token, admin.user); err != nil {
		return err
	}
	return nil
}

func verifyCoreUserInfo(ctx context.Context, client *http.Client, adminURL, token, username string) error {
	if client == nil || strings.TrimSpace(adminURL) == "" || token == "" || username == "" {
		return errors.New("Keycloak token verifier requires authenticated operator session")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(adminURL, "/")+"/realms/master/protocol/openid-connect/userinfo", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Keycloak userinfo verification unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reason := "unspecified"
		challenge := response.Header.Get("WWW-Authenticate")
		for _, code := range []string{"Token verification failed", "Session not active", "User session not found", "User not found", "User disabled", "Client disabled", "insufficient_scope", "invalid_token", "invalid_request"} {
			if strings.Contains(challenge, code) {
				reason = code
				break
			}
		}
		return fmt.Errorf("Keycloak bearer token rejected by OIDC userinfo: HTTP %d (%s)", response.StatusCode, reason)
	}
	var identity struct {
		Subject  string `json:"sub"`
		Username string `json:"preferred_username"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<16)).Decode(&identity); err != nil {
		return errors.New("Keycloak userinfo returned invalid claims")
	}
	if identity.Subject == "" || identity.Username != username {
		return errors.New("Keycloak token subject or operator identity differs from protected credentials")
	}
	return nil
}

// The native Admin REST endpoint validates the actual lightweight token. This
// read also binds the protected operator credential to its enabled master user.
func verifyCoreAdminToken(ctx context.Context, client *http.Client, adminURL, token, username string) error {
	if client == nil || strings.TrimSpace(adminURL) == "" || token == "" || username == "" {
		return errors.New("Keycloak token verifier requires authenticated operator session")
	}
	endpoint := strings.TrimRight(adminURL, "/") + "/admin/realms/master/users?" + url.Values{"username": {username}, "exact": {"true"}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Keycloak native token verification unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Keycloak bearer token rejected by native Admin REST: HTTP %d", response.StatusCode)
	}
	var users []struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<16)).Decode(&users); err != nil || len(users) != 1 || users[0].ID == "" || users[0].Username != username || !users[0].Enabled {
		return errors.New("Keycloak token operator identity differs from enabled protected master user")
	}
	return nil
}
