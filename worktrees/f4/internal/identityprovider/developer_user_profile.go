package identityprovider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
)

// Keycloak ignores undeclared user attributes under its default strict profile.
// Declare only the developer ownership marker, with admin-only view/edit, in
// the verified existing operator realm. Preserve all other profile attributes
// and the unmanaged-attribute policy. No password or permission reset occurs.
func (a *keycloakAdmin) ensureDeveloperOwnershipAttribute(ctx context.Context, realm, target string) error {
	base := "/admin/realms/" + url.PathEscape(realm)
	status, body, err := a.do(ctx, http.MethodGet, base, nil)
	if err != nil {
		return err
	}
	var current keycloakRealm
	ownership := map[string]string{"baseharbor.owner": "baseharbor", "baseharbor.scope": "operator", "baseharbor.target": strings.TrimSpace(target), "baseharbor.environment": "dev"}
	if status != http.StatusOK || json.Unmarshal([]byte(body), &current) != nil || !keycloakRealmOwnedBy(current, ownership) {
		return errors.New("developer ownership attribute requires the verified operator realm owner")
	}
	path := base + "/users/profile"
	status, body, err = a.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	var profile map[string]json.RawMessage
	if status != http.StatusOK || json.Unmarshal([]byte(body), &profile) != nil || profile == nil {
		return errors.New("developer user profile is unverifiable")
	}
	var attributes []map[string]any
	if json.Unmarshal(profile["attributes"], &attributes) != nil {
		return errors.New("developer user profile attributes are unverifiable")
	}
	desired := map[string]any{
		"name": "baseharbor.scope", "multivalued": false,
		"permissions": map[string]any{"view": []any{"admin"}, "edit": []any{"admin"}},
		"validations": map[string]any{"options": map[string]any{"options": []any{"developer-access"}}},
	}
	markers := 0
	for _, attribute := range attributes {
		if attribute["name"] != "baseharbor.scope" {
			continue
		}
		markers++
		if markers > 1 || !reflect.DeepEqual(attribute["permissions"], desired["permissions"]) || !reflect.DeepEqual(attribute["validations"], desired["validations"]) || (attribute["multivalued"] != nil && attribute["multivalued"] != false) {
			return errors.New("existing developer ownership attribute differs; preserve it pending an explicit authorized profile change")
		}
	}
	if markers == 1 {
		return nil
	}
	attributes = append(attributes, desired)
	profile["attributes"], err = json.Marshal(attributes)
	if err != nil {
		return err
	}
	status, _, err = a.do(ctx, http.MethodPut, path, profile)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return errors.New("declare developer ownership attribute failed; existing credentials preserved")
	}
	return nil
}
