package identityprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKeycloakClientSecretOverlapRotation(t *testing.T) {
	var retired bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/client-secret"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"value":"new-secret"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/client-secret"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"value":"new-secret"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/client-secret/rotated"):
			if retired {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"value":"old-secret"}`))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/client-secret/rotated"):
			retired = true
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
	current, rotated, err := admin.rotateClientSecret(context.Background(), "realm", "uuid")
	if err != nil {
		t.Fatal(err)
	}
	if current != "new-secret" || rotated != "old-secret" {
		t.Fatalf("rotation = current %q rotated %q", current, rotated)
	}
	if err := admin.retireRotatedClientSecret(context.Background(), "realm", "uuid"); err != nil {
		t.Fatal(err)
	}
	if !retired {
		t.Fatal("rotated secret was not retired")
	}
}

func TestKeycloakClientSecretRotationWaitsForHAReadConvergence(t *testing.T) {
	postCount := 0
	currentReadCount := 0
	rotatedReadCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/client-secret"):
			postCount++
			_, _ = w.Write([]byte(`{"value":"new-secret"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/client-secret/rotated"):
			rotatedReadCount++
			if rotatedReadCount == 1 {
				// A stale HA member returns the current secret as its overlap value.
				_, _ = w.Write([]byte(`{"value":"new-secret"}`))
				return
			}
			_, _ = w.Write([]byte(`{"value":"old-secret"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/client-secret"):
			currentReadCount++
			if currentReadCount == 1 {
				// A member may briefly fail while the HA cluster is converging.
				http.Error(w, "member is restarting", http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"value":"new-secret"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
	current, rotated, err := admin.rotateClientSecret(context.Background(), "realm", "uuid")
	if err != nil {
		t.Fatal(err)
	}
	if current != "new-secret" || rotated != "old-secret" {
		t.Fatalf("rotation = current %q rotated %q", current, rotated)
	}
	if postCount != 1 {
		t.Fatalf("secret regeneration POST count = %d, want exactly one", postCount)
	}
	if rotatedReadCount != 2 || currentReadCount != 3 {
		t.Fatalf("read counts = current %d rotated %d, want 3 current and 2 rotated reads", currentReadCount, rotatedReadCount)
	}
}

func TestKeycloakClientSecretVerificationDistinguishesInvalidClient(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr bool
	}{
		{name: "authenticated inactive token", status: http.StatusOK, body: `{"active":false}`, wantErr: false},
		{name: "invalid secret", status: http.StatusUnauthorized, body: `{"error":"invalid_client"}`, wantErr: true},
		{name: "invalid secret explicit", status: http.StatusBadRequest, body: `{"error":"invalid_client"}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token/introspect") {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
			err := admin.verifyClientSecretAuthentication(context.Background(), "realm", "client", "secret")
			if (err != nil) != tt.wantErr {
				t.Fatalf("verify error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
