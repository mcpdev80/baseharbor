package identityprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeveloperProfileNamesOptionalWithOwnershipAndPolicyPreserved(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		t.Run(map[bool]string{false: "owned", true: "foreign"}[foreign], func(t *testing.T) {
			target := "local"
			owner := target
			if foreign {
				owner = "another"
			}
			profile := map[string]any{"attributes": []any{
				map[string]any{"name": "username", "required": map[string]any{"roles": []string{"user"}}},
				map[string]any{"name": "email", "required": map[string]any{"roles": []string{"user"}}},
				map[string]any{"name": "firstName", "required": map[string]any{"roles": []string{"user"}}, "annotations": map[string]string{"keep": "yes"}},
				map[string]any{"name": "lastName", "required": map[string]any{"roles": []string{"user"}}},
			}, "unmanagedAttributePolicy": "DISABLED"}
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/users/profile") {
					json.NewEncoder(w).Encode(keycloakRealm{Realm: operatorRealm("dev"), Attributes: map[string]string{"baseharbor.owner": "baseharbor", "baseharbor.scope": "operator", "baseharbor.target": owner, "baseharbor.environment": "dev"}})
					return
				}
				if r.Method == http.MethodPut {
					writes++
					if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
						t.Error(err)
					}
					w.WriteHeader(204)
					return
				}
				json.NewEncoder(w).Encode(profile)
			}))
			defer server.Close()
			admin := keycloakAdmin{endpoint: server.URL, client: server.Client()}
			err := admin.ensureDeveloperProfile(context.Background(), target)
			if foreign {
				if err == nil || writes != 0 {
					t.Fatal("foreign realm policy modified")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if writes != 1 {
				t.Fatal("profile was not updated once")
			}
			if err := admin.ensureDeveloperProfile(context.Background(), target); err != nil || writes != 1 {
				t.Fatal("profile update is not idempotent")
			}
			attrs := profile["attributes"].([]any)
			for _, raw := range attrs {
				attribute := raw.(map[string]any)
				name := attribute["name"].(string)
				_, required := attribute["required"]
				if (name == "username" || name == "email") != required {
					t.Fatalf("wrong required policy for %s", name)
				}
				if name == "firstName" && attribute["annotations"].(map[string]any)["keep"] != "yes" {
					t.Fatal("annotation lost")
				}
			}
			if profile["unmanagedAttributePolicy"] != "DISABLED" {
				t.Fatal("unrelated policy changed")
			}
		})
	}
}
