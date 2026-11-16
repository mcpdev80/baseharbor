package identityprovider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

// Observe the retained account without synchronizing a possibly stale password,
// granting new permissions, or adopting a same-named foreign account. This is
// account/role verification, not proof that a supplied password still works.
func (a *keycloakAdmin) verifyExistingDeveloperUser(ctx context.Context, realm, username string, user keycloakUser, realmAdmin bool) error {
	if user.ID == "" || user.Username != username || !user.Enabled || len(user.Attributes["baseharbor.scope"]) != 1 || user.Attributes["baseharbor.scope"][0] != "developer-access" {
		return errors.New("retained developer account is disabled or its ownership is unverifiable; preserve it and use an explicit authorized account change")
	}
	if !realmAdmin {
		return nil
	}
	base := "/admin/realms/" + url.PathEscape(realm)
	query := url.Values{"clientId": []string{"realm-management"}}
	status, body, err := a.do(ctx, http.MethodGet, base+"/clients?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	var clients []keycloakClient
	if status != http.StatusOK || json.Unmarshal([]byte(body), &clients) != nil || len(clients) != 1 || clients[0].ID == "" {
		return errors.New("retained developer account role authority is unverifiable")
	}
	path := base + "/users/" + url.PathEscape(user.ID) + "/role-mappings/clients/" + url.PathEscape(clients[0].ID) + "/composite"
	status, body, err = a.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	var roles []keycloakRole
	if status != http.StatusOK || json.Unmarshal([]byte(body), &roles) != nil {
		return errors.New("retained developer account effective roles are unverifiable")
	}
	for _, role := range roles {
		if role.Name == "realm-admin" && role.ID != "" {
			return nil
		}
	}
	return errors.New("retained developer account lacks the required effective role; use an explicit authorized account change")
}
