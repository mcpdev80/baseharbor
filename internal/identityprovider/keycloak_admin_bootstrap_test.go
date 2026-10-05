package identityprovider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestKeycloakAdminWriteRetriesExplicitBootstrap(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || string(body) != `{"realm":"demo"}` || r.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("retried request lost method, body or authorization: %s %s", r.Method, body)
		}
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "Bootstrap in progress. Retry in 1 seconds.")
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	admin := keycloakAdmin{endpoint: server.URL, client: server.Client(), token: "token"}
	status, _, err := admin.do(context.Background(), http.MethodPost, "/admin/realms", map[string]string{"realm": "demo"})
	if err != nil || status != http.StatusCreated || attempts != 2 {
		t.Fatalf("bootstrap write = status %d, attempts %d, error %v", status, attempts, err)
	}
}

func TestKeycloakAdminWriteDoesNotRetryOtherResponses(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusForbidden, http.StatusBadRequest} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			attempts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempts++
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "operation rejected")
			}))
			defer server.Close()
			admin := keycloakAdmin{endpoint: server.URL, client: server.Client()}
			got, _, err := admin.do(context.Background(), http.MethodPost, "/admin/realms", nil)
			if err != nil || got != status || attempts != 1 {
				t.Fatalf("write unexpectedly retried: status %d, attempts %d, error %v", got, attempts, err)
			}
		})
	}
}

type ambiguousAdminWriteTransport struct{ attempts int }

func (r *ambiguousAdminWriteTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	r.attempts++
	return nil, errors.New("response lost after request reached server")
}

func TestKeycloakAdminWriteDoesNotRetryAmbiguousTransportFailure(t *testing.T) {
	transport := &ambiguousAdminWriteTransport{}
	admin := keycloakAdmin{endpoint: "https://identity.localhost", client: &http.Client{Transport: transport}}
	_, _, err := admin.do(context.Background(), http.MethodPost, "/admin/realms", nil)
	if err == nil || transport.attempts != 1 {
		t.Fatalf("ambiguous mutation replayed: attempts %d, error %v", transport.attempts, err)
	}
}

func TestKeycloakAdminBootstrapRetryHonorsCancellation(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "Bootstrap in progress. Retry in 1 seconds.")
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	admin := keycloakAdmin{endpoint: server.URL, client: server.Client()}
	_, _, err := admin.do(ctx, http.MethodPost, "/admin/realms", nil)
	if !errors.Is(err, context.DeadlineExceeded) || attempts != 1 || !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("bootstrap retry ignored cancellation: attempts %d, error %v", attempts, err)
	}
}
