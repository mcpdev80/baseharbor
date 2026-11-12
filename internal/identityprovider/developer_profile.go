package identityprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// Development accounts are configured using a username/password, not a person's
// real-name profile. Make only the two optional OIDC profile claims optional in
// the owned dev operator realm; preserve username/email and every other policy.
func (a *keycloakAdmin) ensureDeveloperProfile(ctx context.Context, target string) error {
	if err := a.requireDeveloperRealmOwnership(ctx, target); err != nil {
		return err
	}
	path := "/admin/realms/" + url.PathEscape(operatorRealm("dev")) + "/users/profile"
	status, body, err := a.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("inspect developer user profile: HTTP %d", status)
	}
	profile, changed, err := optionalDeveloperNames([]byte(body))
	if err != nil || !changed {
		return err
	}
	status, _, err = a.do(ctx, http.MethodPut, path, profile)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return fmt.Errorf("configure developer user profile: HTTP %d", status)
	}
	status, body, err = a.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("verify developer user profile: HTTP %d", status)
	}
	_, stillRequired, err := optionalDeveloperNames([]byte(body))
	if err != nil {
		return err
	}
	if stillRequired {
		return errors.New("developer profile still requires real-name claims")
	}
	return nil
}

func optionalDeveloperNames(body []byte) (map[string]json.RawMessage, bool, error) {
	var profile map[string]json.RawMessage
	if err := json.Unmarshal(body, &profile); err != nil {
		return nil, false, err
	}
	var attributes []map[string]json.RawMessage
	if err := json.Unmarshal(profile["attributes"], &attributes); err != nil || len(attributes) == 0 {
		return nil, false, errors.New("developer profile attributes are unavailable")
	}
	changed := false
	for _, attribute := range attributes {
		var name string
		if err := json.Unmarshal(attribute["name"], &name); err != nil {
			return nil, false, errors.New("developer profile has an unnamed attribute")
		}
		if name == "firstName" || name == "lastName" {
			if _, exists := attribute["required"]; exists {
				delete(attribute, "required")
				changed = true
			}
		}
	}
	if changed {
		data, err := json.Marshal(attributes)
		if err != nil {
			return nil, false, err
		}
		profile["attributes"] = data
	}
	return profile, changed, nil
}

func (a *keycloakAdmin) requireDeveloperRealmOwnership(ctx context.Context, target string) error {
	realm := operatorRealm("dev")
	path := "/admin/realms/" + url.PathEscape(realm)
	status, body, err := a.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("inspect developer realm profile ownership: HTTP %d", status)
	}
	var owned keycloakRealm
	expected := map[string]string{"baseharbor.owner": "baseharbor", "baseharbor.scope": "operator", "baseharbor.target": target, "baseharbor.environment": "dev"}
	if json.Unmarshal([]byte(body), &owned) != nil || !keycloakRealmOwnedBy(owned, expected) {
		return errors.New("developer user profile realm is not owned by this Target")
	}
	return nil
}
