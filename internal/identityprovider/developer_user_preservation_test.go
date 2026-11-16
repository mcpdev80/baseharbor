package identityprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApplyPreservesRetainedDeveloperPasswordAndRoles(t *testing.T) {
	mutations := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			mutations++
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/users"):
			w.Write([]byte(`[{"id":"existing","username":"developer","enabled":true,"attributes":{"baseharbor.scope":["developer-access"]}}]`))
		case strings.HasSuffix(r.URL.Path, "/clients"):
			w.Write([]byte(`[{"id":"management","clientId":"realm-management"}]`))
		case strings.HasSuffix(r.URL.Path, "/composite"):
			w.Write([]byte(`[{"id":"admin-role","name":"realm-admin"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
	for _, candidate := range []string{"original-candidate", "stale-bao-candidate"} {
		if err := admin.reconcileUserWithPolicy(context.Background(), "developer", "developer", candidate, true, true); err != nil {
			t.Fatal(err)
		}
	}
	if mutations != 0 {
		t.Fatalf("repeat apply mutated a retained account %d times", mutations)
	}
}

func TestApplyDoesNotAdoptOrRepairUnverifiedDeveloperAccount(t *testing.T) {
	for _, body := range []string{
		`[{"id":"foreign","username":"developer","enabled":true}]`,
		`[{"id":"disabled","username":"developer","enabled":false,"attributes":{"baseharbor.scope":["developer-access"]}}]`,
		`[{"id":"existing","username":"developer","enabled":true,"attributes":{"baseharbor.scope":["developer-access"]}}]`,
		`[{"id":"a","username":"developer"},{"id":"b","username":"developer"}]`,
	} {
		t.Run(body, func(t *testing.T) {
			mutations := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					mutations++
					w.WriteHeader(http.StatusForbidden)
					return
				}
				switch {
				case strings.HasSuffix(r.URL.Path, "/users"):
					w.Write([]byte(body))
				case strings.HasSuffix(r.URL.Path, "/clients"):
					w.Write([]byte(`[{"id":"management"}]`))
				default:
					w.Write([]byte(`[]`))
				}
			}))
			defer server.Close()
			admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
			if err := admin.reconcileUserWithPolicy(context.Background(), "developer", "developer", "candidate", true, true); err == nil {
				t.Fatal("unverified account reported ready")
			}
			if mutations != 0 {
				t.Fatal("apply modified account/password/permissions without explicit authorization")
			}
		})
	}
}
