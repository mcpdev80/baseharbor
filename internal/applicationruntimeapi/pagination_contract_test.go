package applicationruntimeapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCapabilitiesPaginationContract proves the runtime list pagination
// contract (docs/spec/runtime-list-pagination-v1.md) on
// GET /runtime/v1/capabilities: the response always carries the `pagination`
// envelope next to the `capabilities` collection and invalid page parameters
// fail closed with the canonical bad_request envelope.
func TestCapabilitiesPaginationContract(t *testing.T) {
	h, err := New(&fakeSecrets{}, fakeVerifier{app: "alpha", token: "runtime-token"})
	if err != nil {
		t.Fatal(err)
	}
	send := func(target string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Authorization", "Bearer runtime-token")
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		return res
	}
	res := send("/runtime/v1/capabilities")
	if res.Code != http.StatusOK {
		t.Fatalf("capabilities status = %d body=%q", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, want := range []string{`"pagination":{"page":1`, `"page_size":100`, `"total":1`, `"total_pages":1`, `"next_page":null`} {
		if !strings.Contains(body, want) {
			t.Fatalf("capabilities envelope missing %q: %s", want, body)
		}
	}
	if !strings.Contains(body, `"capability":"secrets/v1"`) {
		t.Fatalf("capabilities collection lost: %s", body)
	}
	for _, target := range []string{"/runtime/v1/capabilities?page=0", "/runtime/v1/capabilities?page_size=101"} {
		res := send(target)
		if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), `"code":"bad_request"`) {
			t.Fatalf("%s: status %d body %s", target, res.Code, res.Body.String())
		}
	}
}
