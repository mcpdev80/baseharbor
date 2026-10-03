package identityprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	baseHarborSecretRotationProfile = "baseharbor-client-secret-rotation"
	baseHarborSecretRotationPolicy  = "baseharbor-client-secret-rotation"
)

type keycloakClientProfilesRepresentation struct {
	Profiles []json.RawMessage `json:"profiles,omitempty"`
}

type keycloakClientPoliciesRepresentation struct {
	Policies []json.RawMessage `json:"policies,omitempty"`
}

func (a *keycloakAdmin) ensureClientSecretRotationPolicy(ctx context.Context, realm string) error {
	profile := map[string]any{
		"name":        baseHarborSecretRotationProfile,
		"description": "BaseHarbor overlap-safe managed client secret rotation",
		"executors": []any{
			map[string]any{
				"executor": "secret-rotation",
				"configuration": map[string]any{
					"expiration-period":         int64(2592000),
					"rotated-expiration-period": int64(86400),
					"remaining-rotation-period": int64(0),
				},
			},
		},
	}
	if err := a.upsertClientPolicyObject(ctx, realm, "profiles", baseHarborSecretRotationProfile, profile); err != nil {
		return fmt.Errorf("reconcile Keycloak client-secret rotation profile: %w", err)
	}

	policy := map[string]any{
		"name":        baseHarborSecretRotationPolicy,
		"description": "Apply BaseHarbor client secret overlap to confidential clients in this managed realm",
		"enabled":     true,
		"conditions": []any{
			map[string]any{
				"condition": "client-access-type",
				"configuration": map[string]any{
					"type": []string{"confidential"},
				},
			},
		},
		"profiles": []string{baseHarborSecretRotationProfile},
	}
	if err := a.upsertClientPolicyObject(ctx, realm, "policies", baseHarborSecretRotationPolicy, policy); err != nil {
		return fmt.Errorf("reconcile Keycloak client-secret rotation policy: %w", err)
	}
	return nil
}

func (a *keycloakAdmin) upsertClientPolicyObject(ctx context.Context, realm, kind, managedName string, desired map[string]any) error {
	if kind != "profiles" && kind != "policies" {
		return fmt.Errorf("unsupported Keycloak client-policy object kind %q", kind)
	}
	path := "/admin/realms/" + url.PathEscape(realm) + "/client-policies/" + kind
	status, body, err := a.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("read Keycloak client %s: HTTP %d: %s", kind, status, body)
	}

	var existing []json.RawMessage
	switch kind {
	case "profiles":
		var representation keycloakClientProfilesRepresentation
		if err := json.Unmarshal([]byte(body), &representation); err != nil {
			return fmt.Errorf("decode Keycloak client profiles: %w", err)
		}
		existing = representation.Profiles
	case "policies":
		var representation keycloakClientPoliciesRepresentation
		if err := json.Unmarshal([]byte(body), &representation); err != nil {
			return fmt.Errorf("decode Keycloak client policies: %w", err)
		}
		existing = representation.Policies
	}

	desiredJSON, err := json.Marshal(desired)
	if err != nil {
		return err
	}
	out := make([]json.RawMessage, 0, len(existing)+1)
	replaced := false
	for _, raw := range existing {
		var identity struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &identity); err != nil {
			return fmt.Errorf("decode Keycloak client %s identity: %w", kind, err)
		}
		if strings.TrimSpace(identity.Name) == managedName {
			out = append(out, json.RawMessage(desiredJSON))
			replaced = true
			continue
		}
		out = append(out, raw)
	}
	if !replaced {
		out = append(out, json.RawMessage(desiredJSON))
	}

	var payload any
	if kind == "profiles" {
		payload = keycloakClientProfilesRepresentation{Profiles: out}
	} else {
		payload = keycloakClientPoliciesRepresentation{Policies: out}
	}
	status, body, err = a.do(ctx, http.MethodPut, path, payload)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return fmt.Errorf("write Keycloak client %s: HTTP %d: %s", kind, status, body)
	}
	return nil
}
