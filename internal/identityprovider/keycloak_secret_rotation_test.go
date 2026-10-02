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
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/client-secret/rotated"):
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

func TestKeycloakClientSecretVerificationDistinguishesInvalidClient(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantErr    bool
	}{
		{name: "authenticated but grant unavailable", status: http.StatusBadRequest, body: `{"error":"unauthorized_client"}`, wantErr: false},
		{name: "invalid secret", status: http.StatusUnauthorized, body: `{"error":"invalid_client"}`, wantErr: true},
		{name: "invalid secret explicit", status: http.StatusBadRequest, body: `{"error":"invalid_client"}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token") {
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
