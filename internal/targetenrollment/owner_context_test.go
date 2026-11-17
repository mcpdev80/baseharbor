package targetenrollment

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

func TestOwnerContextRequiresVerifiedAuthorizedScope(t *testing.T) {
	now := time.Now().Add(time.Minute)
	principal := &identity.Principal{Issuer: "https://issuer.example", Subject: "owner", ExpiresAt: &now}
	tenant := &tenancy.Context{TenantID: "11111111-1111-4111-8111-111111111111", ExternalIdentityID: "22222222-2222-4222-8222-222222222222", Roles: []string{"editor"}}
	handler := &HTTPHandler{}
	makeRequest := func(scoped, authenticated bool, method string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://core.example"+OwnerContextPath, nil)
		r.TLS = &tls.ConnectionState{}
		if authenticated {
			r = r.WithContext(identity.WithPrincipal(r.Context(), principal))
		}
		if scoped {
			r = r.WithContext(tenancy.WithContext(r.Context(), tenant))
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if got := makeRequest(false, false, http.MethodGet).Code; got != http.StatusUnauthorized {
		t.Fatalf("anonymous owner-context status = %d", got)
	}
	if got := makeRequest(false, true, http.MethodGet).Code; got != http.StatusForbidden {
		t.Fatalf("unscoped owner-context status = %d", got)
	}
	if got := makeRequest(true, true, http.MethodPost).Code; got != http.StatusMethodNotAllowed {
		t.Fatalf("non-GET owner-context status = %d", got)
	}
	w := makeRequest(true, true, http.MethodGet)
	if w.Code != http.StatusOK {
		t.Fatalf("authorized owner-context status = %d: %s", w.Code, w.Body.String())
	}
	var response struct {
		TenantID string `json:"tenant_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.TenantID != tenant.TenantID {
		t.Fatalf("wrong verified tenant: %s (%v)", w.Body.String(), err)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("owner-context response must not be cached")
	}
}
