package applicationsecretapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/authorization"
)

type fakeDynamicSecrets struct {
	fakeSecrets
	ref          applicationsecret.Reference
	value        []byte
	rotated      []byte
	deleted      bool
	createApp    string
	readApp      string
	rotateApp    string
	deleteDynApp string
}

func (f *fakeDynamicSecrets) CreateDynamic(_ context.Context, app string, value []byte) (applicationsecret.Reference, error) {
	f.createApp = app
	f.value = append([]byte(nil), value...)
	if f.ref == "" {
		f.ref = "baseharbor://secrets/dyn-0123456789abcdef0123456789abcdef"
	}
	return f.ref, nil
}

func (f *fakeDynamicSecrets) ReadDynamic(_ context.Context, app string, ref applicationsecret.Reference) ([]byte, error) {
	f.readApp = app
	f.ref = ref
	return append([]byte(nil), f.value...), nil
}

func (f *fakeDynamicSecrets) RotateDynamic(_ context.Context, app string, ref applicationsecret.Reference, value []byte) error {
	f.rotateApp = app
	f.ref = ref
	f.rotated = append([]byte(nil), value...)
	f.value = append([]byte(nil), value...)
	return nil
}

func (f *fakeDynamicSecrets) DeleteDynamic(_ context.Context, app string, ref applicationsecret.Reference) error {
	f.deleteDynApp = app
	f.ref = ref
	f.deleted = true
	return nil
}

func TestDynamicSecretCreateDoesNotEchoValue(t *testing.T) {
	secrets := &fakeDynamicSecrets{}
	h := mustHandler(t, secrets, fakeOwnership{owned: true})
	const value = "provider-api-key"
	req := authenticatedRequest(http.MethodPost, "/api/v1/apps/demo/secret-refs", `{"value":"`+value+`"}`, authorization.RoleEditor)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	if secrets.createApp != "demo" || string(secrets.value) != value {
		t.Fatalf("unexpected create call app=%q value=%q", secrets.createApp, secrets.value)
	}
	if strings.Contains(res.Body.String(), value) {
		t.Fatal("dynamic secret value leaked into create response")
	}
	if !strings.Contains(res.Body.String(), "baseharbor://secrets/") {
		t.Fatalf("create response missing stable reference: %s", res.Body.String())
	}
	if res.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", res.Header().Get("Cache-Control"))
	}
}

func TestDynamicSecretResolveIsExplicitAndNoStore(t *testing.T) {
	secrets := &fakeDynamicSecrets{value: []byte("provider-api-key")}
	h := mustHandler(t, secrets, fakeOwnership{owned: true})
	ref := "baseharbor://secrets/dyn-0123456789abcdef0123456789abcdef"
	req := authenticatedRequest(http.MethodPost, "/api/v1/apps/demo/secret-refs/resolve", `{"ref":"`+ref+`"}`, authorization.RoleViewer)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "provider-api-key") {
		t.Fatalf("resolve did not return requested value: %s", res.Body.String())
	}
	if res.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", res.Header().Get("Cache-Control"))
	}
}

func TestDynamicSecretRotateKeepsReferenceAndDoesNotEchoValue(t *testing.T) {
	secrets := &fakeDynamicSecrets{}
	h := mustHandler(t, secrets, fakeOwnership{owned: true})
	ref := "baseharbor://secrets/dyn-0123456789abcdef0123456789abcdef"
	const rotated = "rotated-provider-key"
	req := authenticatedRequest(http.MethodPut, "/api/v1/apps/demo/secret-refs/resolve", `{"ref":"`+ref+`","value":"`+rotated+`"}`, authorization.RoleEditor)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	if string(secrets.rotated) != rotated || secrets.ref.String() != ref {
		t.Fatalf("unexpected rotation ref=%q value=%q", secrets.ref, secrets.rotated)
	}
	if strings.Contains(res.Body.String(), rotated) {
		t.Fatal("rotated secret value leaked into response")
	}
	if !strings.Contains(res.Body.String(), ref) {
		t.Fatal("rotation did not preserve stable reference")
	}
}

func TestDynamicSecretDeleteRequiresOwnership(t *testing.T) {
	secrets := &fakeDynamicSecrets{}
	h := mustHandler(t, secrets, fakeOwnership{owned: false})
	ref := "baseharbor://secrets/dyn-0123456789abcdef0123456789abcdef"
	req := authenticatedRequest(http.MethodDelete, "/api/v1/apps/demo/secret-refs/resolve", `{"ref":"`+ref+`"}`, authorization.RoleEditor)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusForbidden)
	}
	if secrets.deleted {
		t.Fatal("dynamic secret delete reached service after ownership denial")
	}
}
