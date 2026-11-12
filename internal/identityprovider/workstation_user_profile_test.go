package identityprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestEnsureWorkstationUserAttributesDeclaresAdminOnlyMetadata(t *testing.T) {
	profile := map[string]any{"attributes": []any{
		map[string]any{
			"name": "baseharbor.scope", "multivalued": false,
			"permissions": map[string]any{"view": []any{"admin"}, "edit": []any{"admin"}},
			"validations": map[string]any{"options": map[string]any{"options": []any{"developer-access"}}},
		},
	}}
	writeCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/bh-operators-dev":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"realm":"bh-operators-dev","enabled":true,"attributes":{"baseharbor.owner":"baseharbor","baseharbor.scope":"operator","baseharbor.target":"isolated","baseharbor.environment":"dev"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/bh-operators-dev/users/profile":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(profile)
		case r.Method == http.MethodPut && r.URL.Path == "/admin/realms/bh-operators-dev/users/profile":
			writeCount++
			if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
				t.Errorf("decode profile update: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	admin := &keycloakAdmin{endpoint: server.URL, client: server.Client(), token: "test"}
	owner := map[string]string{"baseharbor.owner": "baseharbor", "baseharbor.scope": "operator-workstation", "baseharbor.target": "isolated", "baseharbor.environment": "dev"}
	if err := admin.ensureWorkstationUserAttributes(context.Background(), "bh-operators-dev", owner); err != nil {
		t.Fatal(err)
	}
	if writeCount != 1 {
		t.Fatalf("profile writes = %d, want one", writeCount)
	}
	attributes, ok := profile["attributes"].([]any)
	if !ok {
		t.Fatalf("profile attributes have type %T", profile["attributes"])
	}
	wantNames := map[string]bool{"baseharbor.owner": true, "baseharbor.scope": true, "baseharbor.target": true, "baseharbor.environment": true, "baseharbor.display_name": true}
	for _, raw := range attributes {
		attribute := raw.(map[string]any)
		name, _ := attribute["name"].(string)
		delete(wantNames, name)
		if name == "baseharbor.owner" || name == "baseharbor.scope" || name == "baseharbor.target" || name == "baseharbor.environment" || name == "baseharbor.display_name" {
			if !reflect.DeepEqual(attribute["permissions"], map[string]any{"view": []any{"admin"}, "edit": []any{"admin"}}) {
				t.Errorf("%s permissions = %#v", name, attribute["permissions"])
			}
		}
		if name == "baseharbor.scope" {
			validations := attribute["validations"].(map[string]any)
			options := validations["options"].(map[string]any)["options"].([]any)
			if !reflect.DeepEqual(options, []any{"developer-access", "operator-workstation"}) {
				t.Errorf("scope options = %#v", options)
			}
		}
	}
	if len(wantNames) != 0 {
		t.Errorf("missing profile attributes: %v", wantNames)
	}
	if err := admin.ensureWorkstationUserAttributes(context.Background(), "bh-operators-dev", owner); err != nil {
		t.Fatal(err)
	}
	if writeCount != 1 {
		t.Fatalf("idempotent profile writes = %d, want one", writeCount)
	}
}
