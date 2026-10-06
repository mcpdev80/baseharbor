package identityprovider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminReadinessRetriesTransientServerFailure(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count < 3 {
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"actual-test-response"}`))
	}))
	defer server.Close()
	admin := &keycloakAdmin{endpoint: server.URL, client: server.Client(), user: "owner", password: "test-password"}
	if err := waitKeycloakAdminLogin(context.Background(), admin); err != nil || count != 3 || admin.token != "actual-test-response" {
		t.Fatalf("transient readiness: count=%d err=%v", count, err)
	}
}
func TestAdminReadinessDoesNotRetryInvalidCredentials(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { count++; w.WriteHeader(401) }))
	defer server.Close()
	admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
	var status *keycloakAdminLoginError
	if err := waitKeycloakAdminLogin(context.Background(), admin); !errors.As(err, &status) || status.Status != 401 || count != 1 {
		t.Fatalf("invalid credentials replayed: count=%d err=%v", count, err)
	}
}
