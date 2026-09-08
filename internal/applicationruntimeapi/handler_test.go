package applicationruntimeapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
)

type fakeVerifier struct {
	app   string
	token string
}

func (f fakeVerifier) Verify(_ context.Context, app, token string) error {
	if app == f.app && token == f.token {
		return nil
	}
	return context.Canceled
}

type fakeSecrets struct {
	ref   applicationsecret.Reference
	value []byte
}

func (f *fakeSecrets) CreateDynamic(_ context.Context, _ string, value []byte) (applicationsecret.Reference, error) {
	f.value = append([]byte(nil), value...)
	if f.ref == "" {
		f.ref = applicationsecret.Reference("baseharbor://secrets/dyn-0123456789abcdef0123456789abcdef")
	}
	return f.ref, nil
}

func (f *fakeSecrets) ReadDynamic(context.Context, string, applicationsecret.Reference) ([]byte, error) {
	return append([]byte(nil), f.value...), nil
}

func (f *fakeSecrets) RotateDynamic(_ context.Context, _ string, _ applicationsecret.Reference, value []byte) error {
	f.value = append([]byte(nil), value...)
	return nil
}

func (f *fakeSecrets) DeleteDynamic(context.Context, string, applicationsecret.Reference) error {
	f.value = nil
	return nil
}

func TestRuntimeHandlerRequiresMatchingAppToken(t *testing.T) {
	h, err := New(&fakeSecrets{}, fakeVerifier{app: "alpha", token: "runtime-token"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path  string
		token string
	}{
		{path: "/runtime/v1/apps/alpha/secret-refs", token: "wrong"},
		{path: "/runtime/v1/apps/beta/secret-refs", token: "runtime-token"},
	} {
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(`{"value":"secret"}`))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d, want 401", tc.path, res.Code)
		}
	}
}

func TestRuntimeHandlerLifecycleDoesNotEchoMutationValues(t *testing.T) {
	secrets := &fakeSecrets{}
	h, err := New(secrets, fakeVerifier{app: "alpha", token: "runtime-token"})
	if err != nil {
		t.Fatal(err)
	}

	create := request(t, h, http.MethodPost, "/runtime/v1/apps/alpha/secret-refs", `{"value":"first-secret"}`)
	if create.Code != http.StatusCreated || strings.Contains(create.Body.String(), "first-secret") {
		t.Fatalf("create status/body = %d %q", create.Code, create.Body.String())
	}
	if create.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("create response is cacheable")
	}
	ref := secrets.ref.String()

	read := request(t, h, http.MethodPost, "/runtime/v1/apps/alpha/secret-refs/resolve", `{"ref":"`+ref+`"}`)
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), "first-secret") {
		t.Fatalf("read status/body = %d %q", read.Code, read.Body.String())
	}
	if read.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("read response is cacheable")
	}

	rotate := request(t, h, http.MethodPut, "/runtime/v1/apps/alpha/secret-refs/resolve", `{"ref":"`+ref+`","value":"second-secret"}`)
	if rotate.Code != http.StatusOK || strings.Contains(rotate.Body.String(), "second-secret") {
		t.Fatalf("rotate status/body = %d %q", rotate.Code, rotate.Body.String())
	}

	deleteResponse := request(t, h, http.MethodDelete, "/runtime/v1/apps/alpha/secret-refs/resolve", `{"ref":"`+ref+`"}`)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", deleteResponse.Code)
	}
}

func request(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer runtime-token")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}
