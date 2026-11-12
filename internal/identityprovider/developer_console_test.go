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

func TestDeveloperConsoleOriginsAndFramesPreserveNativeClientSettings(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		t.Run(map[bool]string{false: "owned", true: "foreign"}[foreign], func(t *testing.T) {
			writes := 0
			headers := map[string]string{"contentSecurityPolicy": "frame-src 'self'; frame-ancestors 'self'; object-src 'none'; script-src 'nonce-keep';", "xFrameOptions": "SAMEORIGIN"}
			clients := map[string]map[string]any{}
			for _, id := range []string{"account-console", "security-admin-console"} {
				clients[id] = map[string]any{"id": id, "clientId": id, "rootUrl": "${authBaseUrl}", "webOrigins": []string{}, "redirectUris": []string{"/native/*"}, "unknownNativeSetting": "preserve"}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				root := "/admin/realms/" + operatorRealm("dev")
				if r.URL.Path == root {
					if r.Method == http.MethodPut {
						var patch struct {
							BrowserSecurityHeaders map[string]string `json:"browserSecurityHeaders"`
						}
						json.NewDecoder(r.Body).Decode(&patch)
						headers = patch.BrowserSecurityHeaders
						writes++
						w.WriteHeader(204)
						return
					}
					target := "local"
					if foreign {
						target = "foreign"
					}
					json.NewEncoder(w).Encode(map[string]any{"attributes": map[string]string{"baseharbor.owner": "baseharbor", "baseharbor.scope": "operator", "baseharbor.target": target, "baseharbor.environment": "dev"}, "browserSecurityHeaders": headers})
					return
				}
				id := r.URL.Query().Get("clientId")
				if id != "" {
					json.NewEncoder(w).Encode([]any{clients[id]})
					return
				}
				id = strings.TrimPrefix(r.URL.Path, root+"/clients/")
				if r.Method == http.MethodPut {
					var updated map[string]any
					json.NewDecoder(r.Body).Decode(&updated)
					clients[id] = updated
					writes++
					w.WriteHeader(204)
					return
				}
				json.NewEncoder(w).Encode(clients[id])
			}))
			defer server.Close()
			admin := keycloakAdmin{endpoint: server.URL, client: server.Client()}
			origins := []string{"https://auth.example.test:18444", "https://192.0.2.10:52341"}
			err := admin.ensureDeveloperConsoleOrigins(context.Background(), "local", origins)
			if foreign {
				if err == nil || writes != 0 {
					t.Fatal("foreign realm changed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if writes != 3 {
				t.Fatalf("expected frame policy and two client changes, got %d", writes)
			}
			for _, c := range clients {
				var got []string
				raw, _ := json.Marshal(c["webOrigins"])
				json.Unmarshal(raw, &got)
				if !reflect.DeepEqual(got, origins) || c["unknownNativeSetting"] != "preserve" || c["rootUrl"] != "${authBaseUrl}" {
					t.Fatal("native client settings lost")
				}
			}
			for _, keep := range []string{"frame-ancestors 'self'", "object-src 'none'", "script-src 'nonce-keep'"} {
				if !strings.Contains(headers["contentSecurityPolicy"], keep) {
					t.Fatal("unrelated CSP modified")
				}
			}
			if headers["xFrameOptions"] != "SAMEORIGIN" {
				t.Fatal("unrelated security header modified")
			}
			if err := admin.ensureDeveloperConsoleOrigins(context.Background(), "local", origins); err != nil || writes != 3 {
				t.Fatalf("not idempotent: %v %d", err, writes)
			}
		})
	}
}

func TestDeveloperConsoleOriginSelectionUsesActualPublishedPorts(t *testing.T) {
	got, err := developmentConsoleOrigins(map[string]string{"identity": "https://auth.example.test:19444/realms/dev/account/", "sql": "https://unrelated.test:555"}, map[string]string{"identity_admin": "https://192.0.2.10:55123/admin/dev/console/"})
	if err != nil || !reflect.DeepEqual(got, []string{"https://192.0.2.10:55123", "https://auth.example.test:19444"}) {
		t.Fatalf("invalid published origins: %v %v", got, err)
	}
	for _, bad := range []string{"http://auth.test/", "https://user:secret@auth.test/", "https://auth.test/?query=yes"} {
		if _, err := developmentConsoleOrigins(map[string]string{"identity": bad}); err == nil {
			t.Fatal("invalid endpoint accepted")
		}
	}
	policy := developerConsoleFramePolicy("frame-src 'self' https://old.test; object-src 'none';", got)
	if strings.Contains(policy, "old.test") || !strings.Contains(policy, "object-src 'none'") {
		t.Fatal("stale managed frame origin retained")
	}
}
