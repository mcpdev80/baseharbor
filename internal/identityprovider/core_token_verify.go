package identityprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// VerifyCoreOperatorTokenFlow performs a real OAuth password grant against
// the installation-owned Keycloak admin identity and requires an authenticated
// userinfo response over the provisioned TLS trust. The Core realm, ownership
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
	if err := verifyCoreUserInfo(ctx, admin.client, files.AdminURL, admin.token, admin.user); err != nil {
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
		return errors.New("Keycloak bearer token rejected by OIDC userinfo")
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
