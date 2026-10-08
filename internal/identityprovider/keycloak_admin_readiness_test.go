package identityprovider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

func TestAdminReadinessRetriesNativeUnexpectedServerException(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count == 1 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"unauthorized_client","error_description":"Unexpected error when authenticating client"}`))
			return
		}
		w.Write([]byte(`{"access_token":"recovered"}`))
	}))
	defer server.Close()
	admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
	if err := waitKeycloakAdminLogin(context.Background(), admin); err != nil || count != 2 || admin.token != "recovered" {
		t.Fatalf("native token readiness did not recover: count=%d err=%v", count, err)
	}
}

func TestAdminReadinessDoesNotRetryOtherOAuthClientErrors(t *testing.T) {
	for _, body := range []string{
		`{"error":"invalid_grant","error_description":"Invalid user credentials"}`,
		`{"error":"unauthorized_client","error_description":"Client credentials setup required"}`,
		`{"error":"invalid_client","error_description":"Unexpected error when authenticating client"}`,
		`not-json`,
	} {
		t.Run(body, func(t *testing.T) {
			count := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count++
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(body))
			}))
			defer server.Close()
			admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
			if err := waitKeycloakAdminLogin(context.Background(), admin); err == nil || count != 1 {
				t.Fatalf("rejected OAuth client was retried: count=%d err=%v", count, err)
			}
		})
	}
}

func TestAdminReadinessPersistentNativeFailureHonorsDeadline(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"unauthorized_client","error_description":"Unexpected error when authenticating client"}`))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
	if err := waitKeycloakAdminLogin(ctx, admin); !errors.Is(err, context.DeadlineExceeded) || count != 1 {
		t.Fatalf("native readiness ignored deadline: count=%d err=%v", count, err)
	}
}
