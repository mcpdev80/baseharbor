package identityprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

const ownedDeveloperRealm = `{"realm":"developer","attributes":{"baseharbor.owner":"baseharbor","baseharbor.scope":"operator","baseharbor.target":"isolated","baseharbor.environment":"dev"}}`

func TestDeveloperOwnershipAttributePreservesStrictProfileAndExistingAttributes(t *testing.T) {
	profile := `{"attributes":[{"name":"username","permissions":{"view":["admin","user"],"edit":["admin","user"]}}],"groups":[{"name":"existing-group"}],"unmanagedAttributePolicy":"DISABLED"}`
	var original map[string]any
	if err := json.Unmarshal([]byte(profile), &original); err != nil {
		t.Fatal(err)
	}
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/users/profile") {
			if r.Method == http.MethodPut {
				writes++
				var update map[string]any
				if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
					t.Error(err)
				}
				if update["unmanagedAttributePolicy"] != original["unmanagedAttributePolicy"] || !reflect.DeepEqual(update["groups"], original["groups"]) {
					t.Error("profile update changed unrelated policy or groups")
				}
				attrs := update["attributes"].([]any)
				if len(attrs) != 2 || !reflect.DeepEqual(attrs[0], original["attributes"].([]any)[0]) {
					t.Error("profile update changed unrelated attributes")
				}
				marker := attrs[1].(map[string]any)
				permissions := marker["permissions"].(map[string]any)
				if !reflect.DeepEqual(permissions["edit"], []any{"admin"}) || !reflect.DeepEqual(permissions["view"], []any{"admin"}) {
					t.Error("user can edit or view the ownership marker")
				}
				// Harmless server metadata must not cause a rewrite on re-add.
				marker["annotations"] = map[string]any{}
				data, _ := json.Marshal(update)
				profile = string(data)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.Write([]byte(profile))
			return
		}
		w.Write([]byte(ownedDeveloperRealm))
	}))
	defer server.Close()
	admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
	for range 2 {
		if err := admin.ensureDeveloperOwnershipAttribute(context.Background(), "developer", "isolated"); err != nil {
			t.Fatal(err)
		}
	}
	if writes != 1 {
		t.Fatalf("profile mutated on repeat apply: %d writes", writes)
	}
}

func TestDeveloperOwnershipAttributeDoesNotAdoptForeignOrUserEditableProfile(t *testing.T) {
	for _, foreignRealm := range []bool{false, true} {
		t.Run(map[bool]string{false: "user-editable-marker", true: "foreign-realm"}[foreignRealm], func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes++
					w.WriteHeader(http.StatusForbidden)
					return
				}
				if strings.HasSuffix(r.URL.Path, "/users/profile") {
					w.Write([]byte(`{"attributes":[{"name":"baseharbor.scope","permissions":{"edit":["user"]}}]}`))
				} else if foreignRealm {
					w.Write([]byte(`{"realm":"developer","attributes":{}}`))
				} else {
					w.Write([]byte(ownedDeveloperRealm))
				}
			}))
			defer server.Close()
			admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
			if err := admin.ensureDeveloperOwnershipAttribute(context.Background(), "developer", "isolated"); err == nil || writes != 0 {
				t.Fatal("unverified or user-editable ownership was adopted")
			}
		})
	}
}
