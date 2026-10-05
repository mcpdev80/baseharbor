package identityprovider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestKeycloakAdminLoginRecoversFromDatabaseReloadErrors(t *testing.T) {
	for _, status := range []int{500, 502, 503, 504} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			attempts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					w.Write([]byte(`{}`))
					return
				}
				attempts++
				if attempts == 1 {
					w.WriteHeader(status)
					w.Write([]byte(`{"error":"unknown_error"}`))
					return
				}
				w.Write([]byte(`{"access_token":"ready"}`))
			}))
			defer server.Close()
			driver := &KeycloakDriver{instance: KeycloakInstance{EndpointBaseURL: server.URL, AdminHTTPClient: server.Client()}}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			admin, err := driver.adminClient(ctx)
			if err != nil || admin.token != "ready" || attempts != 2 {
				t.Fatalf("admin=%v err=%v attempts=%d", admin, err, attempts)
			}
		})
	}
}

func TestKeycloakAdminLoginDoesNotRetryRejectedCredentials(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Write([]byte(`{}`))
			return
		}
		attempts++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	driver := &KeycloakDriver{instance: KeycloakInstance{EndpointBaseURL: server.URL, AdminHTTPClient: server.Client()}}
	_, err := driver.adminClient(context.Background())
	var loginErr *keycloakAdminLoginError
	if !errors.As(err, &loginErr) || loginErr.Status != http.StatusUnauthorized || attempts != 1 {
		t.Fatalf("err=%v attempts=%d", err, attempts)
	}
}
