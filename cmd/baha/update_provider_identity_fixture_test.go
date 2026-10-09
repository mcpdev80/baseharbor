package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Seed and verify real provider-owned realms/clients/users/roles via Keycloak's
// authenticated API. No HTTP handler or provider operation is replaced.
func providerFixtureIdentity(t *testing.T, ctx context.Context, o *coreNativeRuntimeOps) (check func(), changeUser func()) {
	t.Helper()
	values, err := bhruntime.RuntimeEnvironment(bhruntime.Files{Env: o.identity.Env})
	if err != nil {
		t.Fatal(err)
	}
	pem, err := os.ReadFile(o.identity.PublicAccess.Material.CA)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		t.Fatal("Keycloak fixture CA unavailable")
	}
	issuer, err := url.Parse(o.issuer)
	if err != nil || issuer.Hostname() == "" {
		t.Fatal("Keycloak owned issuer unavailable")
	}
	client := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: issuer.Hostname()}}}
	t.Cleanup(client.CloseIdleConnections)
	base := strings.TrimRight(o.identity.AdminURL, "/")
	token := func(realm, clientID, user, password string) string {
		body := url.Values{"grant_type": {"password"}, "client_id": {clientID}, "username": {user}, "password": {password}, "scope": {"openid profile email"}}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/realms/"+realm+"/protocol/openid-connect/token", strings.NewReader(body.Encode()))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response, err := client.Do(req)
		if err != nil {
			t.Fatal("Keycloak fixture token transport failed")
		}
		defer response.Body.Close()
		var result struct {
			AccessToken string `json:"access_token"`
		}
		if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result) != nil || result.AccessToken == "" {
			t.Fatal("Keycloak fixture actual token issuance failed")
		}
		return result.AccessToken
	}
	admin := func(method, path string, body any, want int) (http.Header, []byte) {
		access := token("master", "admin-cli", values["BASEHARBOR_KEYCLOAK_ADMIN_USER"], values["BASEHARBOR_KEYCLOAK_ADMIN_PASSWORD"])
		var reader io.Reader
		if body != nil {
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			reader = bytes.NewReader(data)
		}
		req, err := http.NewRequestWithContext(ctx, method, base+"/admin/realms/baseharbor"+path, reader)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set("Content-Type", "application/json")
		response, err := client.Do(req)
		if err != nil {
			t.Fatal("Keycloak fixture admin transport failed")
		}
		defer response.Body.Close()
		data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != want {
			t.Fatalf("Keycloak fixture admin %s %s status=%d, expected=%d", method, path, response.StatusCode, want)
		}
		return response.Header, data
	}
	const clientID = "provider-acceptance-client"
	const username = "provider-acceptance-user"
	const password = "native-isolated-identity-fixture-password"
	const role = "provider-acceptance-role"
	admin(http.MethodPost, "/clients", map[string]any{"clientId": clientID, "enabled": true, "protocol": "openid-connect", "publicClient": true, "directAccessGrantsEnabled": true, "standardFlowEnabled": true, "defaultClientScopes": []string{"profile", "email", "roles"}}, http.StatusCreated)
	admin(http.MethodPost, "/roles", map[string]any{"name": role}, http.StatusCreated)
	header, _ := admin(http.MethodPost, "/users", map[string]any{"username": username, "enabled": true, "firstName": "Fixture", "lastName": "Acceptance", "email": "fixture@example.invalid", "emailVerified": true, "credentials": []map[string]any{{"type": "password", "value": password, "temporary": false}}}, http.StatusCreated)
	location, err := url.Parse(header.Get("Location"))
	if err != nil {
		t.Fatal("Keycloak user identity missing")
	}
	userID := location.Path[strings.LastIndex(location.Path, "/")+1:]
	if userID == "" {
		t.Fatal("Keycloak user identity missing")
	}
	_, roleJSON := admin(http.MethodGet, "/roles/"+role, nil, http.StatusOK)
	var roleObject map[string]any
	if json.Unmarshal(roleJSON, &roleObject) != nil || roleObject["name"] != role {
		t.Fatal("Keycloak role creation failed")
	}
	admin(http.MethodPost, "/users/"+userID+"/role-mappings/realm", []map[string]any{roleObject}, http.StatusNoContent)
	check = func() {
		access := token("baseharbor", clientID, username, password)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/realms/baseharbor/protocol/openid-connect/userinfo", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+access)
		response, err := client.Do(req)
		if err != nil {
			t.Fatal("Keycloak fixture actual userinfo transport failed")
		}
		var identity struct {
			Subject  string `json:"sub"`
			Username string `json:"preferred_username"`
		}
		err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&identity)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || err != nil || identity.Subject != userID || identity.Username != username {
			t.Fatal("Keycloak fixture token/user identity not preserved")
		}
		_, data := admin(http.MethodGet, "/users/"+userID+"/role-mappings/realm", nil, http.StatusOK)
		var mappings []map[string]any
		if json.Unmarshal(data, &mappings) != nil {
			t.Fatal("Keycloak role mapping response invalid")
		}
		found := false
		for _, m := range mappings {
			if m["name"] == role && m["id"] == roleObject["id"] {
				found = true
			}
		}
		if !found {
			t.Fatal("Keycloak native user role mapping lost")
		}
		_, data = admin(http.MethodGet, "/clients?clientId="+url.QueryEscape(clientID), nil, http.StatusOK)
		var clients []map[string]any
		if json.Unmarshal(data, &clients) != nil || len(clients) != 1 || clients[0]["clientId"] != clientID {
			t.Fatal("Keycloak native client not preserved")
		}
	}
	changeUser = func() {
		admin(http.MethodPut, "/users/"+userID, map[string]any{"username": username + "-after-backup", "enabled": true}, http.StatusNoContent)
	}
	check()
	t.Log("Native Keycloak client, user, role mapping, OAuth token and authenticated userinfo fixture verified")
	return check, changeUser
}
