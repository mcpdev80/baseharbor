package runtimeapidocs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesEmbeddedOpenAPI(t *testing.T) {
	h := Handler()
	req := httptest.NewRequest(http.MethodGet, OpenAPIPath, nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("openapi status = %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "openapi: 3.1.0") ||
		!strings.Contains(res.Body.String(), "/runtime/v1/resources") {
		t.Fatalf("unexpected OpenAPI document: %s", res.Body.String())
	}
	if got := res.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
}

func TestHandlerServesEmbeddedSwaggerUI(t *testing.T) {
	h := Handler()
	req := httptest.NewRequest(http.MethodGet, UIPath, nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("swagger status = %d body=%s", res.Code, res.Body.String())
	}
	body := strings.ToLower(res.Body.String())
	if !strings.Contains(body, "swagger") || !strings.Contains(body, "openapi.yaml") {
		t.Fatalf("Swagger UI does not reference embedded API contract: %s", res.Body.String())
	}
}
