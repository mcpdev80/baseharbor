package identityprovider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
)

// Keycloak's strict user profile drops undeclared attributes from admin API
// user representations. Declare the ownership metadata before creating users,
// and keep it visible/editable only to administrators.
func (a *keycloakAdmin) ensureWorkstationUserAttributes(ctx context.Context, realm string, ownership map[string]string) error {
	base := "/admin/realms/" + url.PathEscape(realm)
	status, body, err := a.do(ctx, http.MethodGet, base, nil)
	if err != nil {
		return err
	}
	var current keycloakRealm
	if status != http.StatusOK || json.Unmarshal([]byte(body), &current) != nil || !keycloakRealmOwnedBy(current, map[string]string{
		"baseharbor.owner": "baseharbor", "baseharbor.scope": "operator",
		"baseharbor.target": ownership["baseharbor.target"], "baseharbor.environment": ownership["baseharbor.environment"],
	}) {
		return errors.New("workstation user attributes require the verified operator realm owner")
	}
	path := base + "/users/profile"
	status, body, err = a.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	var profile map[string]json.RawMessage
	if status != http.StatusOK || json.Unmarshal([]byte(body), &profile) != nil || profile == nil {
		return errors.New("workstation user profile is unverifiable")
	}
	var attributes []map[string]any
	if json.Unmarshal(profile["attributes"], &attributes) != nil {
		return errors.New("workstation user profile attributes are unverifiable")
	}
	permissions := map[string]any{"view": []any{"admin"}, "edit": []any{"admin"}}
	metadata := []string{"baseharbor.owner", "baseharbor.scope", "baseharbor.target", "baseharbor.environment", "baseharbor.display_name"}
	changed := false
	for _, name := range metadata {
		matches := 0
		for _, attribute := range attributes {
			if attribute["name"] != name {
				continue
			}
			matches++
			if matches > 1 || !reflect.DeepEqual(attribute["permissions"], permissions) || (attribute["multivalued"] != nil && attribute["multivalued"] != false) {
				return errors.New("existing workstation ownership profile differs; preserve it pending explicit review")
			}
		}
		if matches == 0 {
			attributes = append(attributes, map[string]any{
				"name": name, "multivalued": false,
				"permissions": permissions,
			})
			changed = true
		}
	}
	// The shared operator realm may also contain the existing developer-access
	// account. Preserve that allowed scope while adding the workstation scope.
	for _, attribute := range attributes {
		if attribute["name"] != "baseharbor.scope" {
			continue
		}
		validations, _ := attribute["validations"].(map[string]any)
		optionsValidation, _ := validations["options"].(map[string]any)
		options, _ := optionsValidation["options"].([]any)
		seen := map[string]bool{}
		for _, option := range options {
			if value, ok := option.(string); ok {
				seen[value] = true
			}
		}
		if !seen["operator-workstation"] {
			options = append(options, "operator-workstation")
			if validations == nil {
				validations = map[string]any{}
			}
			if optionsValidation == nil {
				optionsValidation = map[string]any{}
			}
			optionsValidation["options"] = options
			validations["options"] = optionsValidation
			attribute["validations"] = validations
			changed = true
		}
	}
	if !changed {
		return nil
	}
	profile["attributes"], err = json.Marshal(attributes)
	if err != nil {
		return err
	}
	status, _, err = a.do(ctx, http.MethodPut, path, profile)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return errors.New("declare workstation ownership attributes failed")
	}
	status, body, err = a.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK || json.Unmarshal([]byte(body), &profile) != nil {
		return errors.New("verify workstation ownership attributes failed")
	}
	var verified []map[string]any
	if json.Unmarshal(profile["attributes"], &verified) != nil {
		return errors.New("verified workstation user profile is unreadable")
	}
	for _, name := range metadata {
		count := 0
		for _, attribute := range verified {
			if attribute["name"] == name && reflect.DeepEqual(attribute["permissions"], permissions) {
				count++
			}
		}
		if count != 1 {
			return errors.New("workstation ownership profile did not retain its admin-only attributes")
		}
	}
	return nil
}
