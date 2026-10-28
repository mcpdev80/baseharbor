package applicationsecretapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/authorization"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

type fakeSecrets struct {
	items      []applicationsecret.Metadata
	setApp     string
	setName    string
	setValue   []byte
	deleteApp  string
	deleteName string
}

func (f *fakeSecrets) List(context.Context, string) ([]applicationsecret.Metadata, error) {
	return append([]applicationsecret.Metadata(nil), f.items...), nil
}

func (f *fakeSecrets) Set(_ context.Context, app, name string, value []byte) error {
	f.setApp = app
	f.setName = name
	f.setValue = append([]byte(nil), value...)
	return nil
}

func (f *fakeSecrets) Delete(_ context.Context, app, name string) error {
	f.deleteApp = app
	f.deleteName = name
	return nil
}

type fakeOwnership struct {
	owned bool
	err   error
}

func (f fakeOwnership) OwnedByTenant(context.Context, string, string) (bool, error) {
	return f.owned, f.err
}

func TestHandlerRequiresAuthentication(t *testing.T) {
	h := mustHandler(t, &fakeSecrets{}, fakeOwnership{owned: true})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/demo/secrets", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusUnauthorized)
	}
}

func TestHandlerRequiresTenantContext(t *testing.T) {
	h := mustHandler(t, &fakeSecrets{}, fakeOwnership{owned: true})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/demo/secrets", nil)
	req = req.WithContext(identity.WithPrincipal(req.Context(), &identity.Principal{Issuer: "issuer", Subject: "subject"}))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusForbidden)
	}
}

func TestViewerCannotSetSecret(t *testing.T) {
	secrets := &fakeSecrets{}
	h := mustHandler(t, secrets, fakeOwnership{owned: true})
	req := authenticatedRequest(http.MethodPut, "/api/v1/apps/demo/secrets/API_TOKEN", `{"value":"should-not-be-written"}`, authorization.RoleViewer)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusForbidden)
	}
	if len(secrets.setValue) != 0 {
		t.Fatal("secret service was called after authorization denial")
	}
}

func TestCrossTenantOwnershipDeniesAccess(t *testing.T) {
	secrets := &fakeSecrets{}
	h := mustHandler(t, secrets, fakeOwnership{owned: false})
	req := authenticatedRequest(http.MethodGet, "/api/v1/apps/demo/secrets", "", authorization.RoleViewer)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusForbidden)
	}
}

func TestSetSecretDoesNotEchoValue(t *testing.T) {
	secrets := &fakeSecrets{}
	h := mustHandler(t, secrets, fakeOwnership{owned: true})
	const value = "super-secret-value"
	req := authenticatedRequest(http.MethodPut, "/api/v1/apps/demo/secrets/API_TOKEN", `{"value":"`+value+`"}`, authorization.RoleEditor)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	if string(secrets.setValue) != value {
		t.Fatal("secret value was not passed to the service unchanged")
	}
	if strings.Contains(res.Body.String(), value) {
		t.Fatal("secret value leaked into API response")
	}
	var response struct {
		Name       string `json:"name"`
		Configured bool   `json:"configured"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Name != "API_TOKEN" || !response.Configured {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestListReturnsMetadataOnly(t *testing.T) {
	secrets := &fakeSecrets{items: []applicationsecret.Metadata{{Name: "API_TOKEN", Required: true, Present: true, Usable: true}}}
	h := mustHandler(t, secrets, fakeOwnership{owned: true})
	req := authenticatedRequest(http.MethodGet, "/api/v1/apps/demo/secrets", "", authorization.RoleViewer)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, want := range []string{"API_TOKEN", `"required":true`, `"present":true`, `"usable":true`} {
		if !strings.Contains(body, want) {
			t.Fatalf("response %q missing %q", body, want)
		}
	}
	if strings.Contains(body, "value") {
		t.Fatal("list response contains a value field")
	}
}

func TestDeleteRequiresOwnershipAndReturnsNoContent(t *testing.T) {
	secrets := &fakeSecrets{}
	h := mustHandler(t, secrets, fakeOwnership{owned: true})
	req := authenticatedRequest(http.MethodDelete, "/api/v1/apps/demo/secrets/API_TOKEN", "", authorization.RoleEditor)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	if secrets.deleteApp != "demo" || secrets.deleteName != "API_TOKEN" {
		t.Fatalf("unexpected delete call: app=%q name=%q", secrets.deleteApp, secrets.deleteName)
	}
}

func mustHandler(t *testing.T, secrets SecretService, ownership OwnershipResolver) *Handler {
	t.Helper()
	h, err := New(secrets, ownership, authorization.NewService())
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func authenticatedRequest(method, target, body, role string) *http.Request {
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, target, reader)
	ctx := identity.WithPrincipal(req.Context(), &identity.Principal{Issuer: "issuer", Subject: "subject"})
	ctx = tenancy.WithContext(ctx, &tenancy.Context{TenantID: "tenant-a", ExternalIdentityID: "identity-a", Roles: []string{role}})
	return req.WithContext(ctx)
}
