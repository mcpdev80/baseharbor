package identityprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// EnsureDevelopmentConsoleOrigins binds Keycloak's built-in browser clients to
// the actual published GUI addresses. No wildcard, issuer or role change is
// needed when the development gateway has a different origin from Core.
func EnsureDevelopmentConsoleOrigins(ctx context.Context, dataDir, target string, endpointMaps ...map[string]string) error {
	origins, err := developmentConsoleOrigins(endpointMaps...)
	if err != nil {
		return err
	}
	if len(origins) == 0 {
		return nil
	}
	app := application.Manifest{Version: application.CurrentVersion, Name: "operator-access", Environment: "dev", Services: application.Services{Identity: true}}
	files, err := ExistingKeycloakFilesAt(app, dataDir, target)
	if err != nil {
		return err
	}
	admin, err := operatorKeycloakAdmin(ctx, files)
	if err != nil {
		return err
	}
	return admin.ensureDeveloperConsoleOrigins(ctx, target, origins)
}

func developmentConsoleOrigins(endpointMaps ...map[string]string) ([]string, error) {
	var origins []string
	for _, endpoints := range endpointMaps {
		for _, key := range []string{"identity", "identity_admin"} {
			raw := endpoints[key]
			if raw == "" {
				continue
			}
			u, err := url.Parse(raw)
			if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return nil, errors.New("developer console URL must be a published HTTPS endpoint")
			}
			origin := u.Scheme + "://" + u.Host
			if !slices.Contains(origins, origin) {
				origins = append(origins, origin)
			}
		}
	}
	slices.Sort(origins)
	return origins, nil
}

func (a *keycloakAdmin) ensureDeveloperConsoleOrigins(ctx context.Context, target string, origins []string) error {
	if err := a.requireDeveloperRealmOwnership(ctx, target); err != nil {
		return err
	}
	if err := a.ensureDeveloperConsoleFrames(ctx, origins); err != nil {
		return err
	}
	root := "/admin/realms/" + url.PathEscape(operatorRealm("dev")) + "/clients"
	for _, id := range []string{"account-console", "security-admin-console"} {
		status, body, err := a.do(ctx, http.MethodGet, root+"?clientId="+url.QueryEscape(id), nil)
		if err != nil {
			return err
		}
		var clients []map[string]json.RawMessage
		if status != http.StatusOK || json.Unmarshal([]byte(body), &clients) != nil || len(clients) != 1 {
			return fmt.Errorf("developer console client %s is unavailable or ambiguous", id)
		}
		client := clients[0]
		var uuid, actualID string
		if json.Unmarshal(client["id"], &uuid) != nil || uuid == "" || json.Unmarshal(client["clientId"], &actualID) != nil || actualID != id {
			return errors.New("developer console client identity mismatch")
		}
		var current, currentRedirects []string
		if err := json.Unmarshal(client["webOrigins"], &current); err != nil && string(client["webOrigins"]) != "" {
			return err
		}
		path := "/realms/" + operatorRealm("dev") + "/account/*"
		if id == "security-admin-console" {
			path = "/admin/" + operatorRealm("dev") + "/console/*"
		}
		redirects := make([]string, 0, len(origins))
		for _, origin := range origins {
			redirects = append(redirects, origin+path)
		}
		if err := json.Unmarshal(client["redirectUris"], &currentRedirects); err != nil {
			return err
		}
		// Keycloak may return client origins in a different order from the
		// request. These are sets for redirect/CORS matching, so compare them
		// without depending on the provider's serialization order.
		if equalStringSets(current, origins) && equalStringSets(currentRedirects, redirects) {
			continue
		}
		// Keycloak's built-in admin console deliberately uses the dynamic
		// ${authAdminUrl} root and relative redirect URI. That configuration
		// follows the incoming HTTPS host through the development gateway; the
		// native "+" web-origin setting derives origins from those redirects.
		// Replacing it with absolute URLs is ignored by Keycloak and makes every
		// subsequent reconciliation fail its read-back check.
		var rootURL string
		_ = json.Unmarshal(client["rootUrl"], &rootURL)
		if id == "security-admin-console" && rootURL == "${authAdminUrl}" &&
			len(current) == 1 && current[0] == "+" &&
			slices.Equal(currentRedirects, []string{path}) {
			continue
		}
		client["redirectUris"], err = json.Marshal(redirects)
		if err != nil {
			return err
		}
		client["webOrigins"], err = json.Marshal(origins)
		if err != nil {
			return err
		}
		status, _, err = a.do(ctx, http.MethodPut, root+"/"+url.PathEscape(uuid), client)
		if err != nil {
			return err
		}
		if status != http.StatusNoContent {
			return fmt.Errorf("configure developer console %s origins: HTTP %d", id, status)
		}
		status, body, err = a.do(ctx, http.MethodGet, root+"/"+url.PathEscape(uuid), nil)
		if err != nil {
			return err
		}
		var observed struct {
			WebOrigins   []string `json:"webOrigins"`
			RedirectURIs []string `json:"redirectUris"`
		}
		if status != http.StatusOK || json.Unmarshal([]byte(body), &observed) != nil || !equalStringSets(observed.WebOrigins, origins) || !equalStringSets(observed.RedirectURIs, redirects) {
			return errors.New("developer console origins did not converge")
		}
	}
	return nil
}

func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	left, right := slices.Clone(a), slices.Clone(b)
	slices.Sort(left)
	slices.Sort(right)
	return slices.Equal(left, right)
}
