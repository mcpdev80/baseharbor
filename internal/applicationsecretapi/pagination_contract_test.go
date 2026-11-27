package applicationsecretapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/authorization"
)

// TestListPaginationContract proves the runtime list pagination contract
// (docs/spec/runtime-list-pagination-v1.md) on GET /api/v1/apps/{app}/secrets:
// the response always carries the `pagination` envelope as a sibling of the
// item collection, page parameters window the collection, and invalid
// parameters fail closed with the canonical bad_request envelope.
func TestListPaginationContract(t *testing.T) {
	const total = 250
	items := make([]applicationsecret.Metadata, total)
	for i := range items {
		items[i] = applicationsecret.Metadata{Name: fmt.Sprintf("TOKEN_%03d", i), Required: true, Present: true, Usable: true}
	}
	h := mustHandler(t, &fakeSecrets{items: items}, fakeOwnership{owned: true})

	send := func(target string) *httptest.ResponseRecorder {
		t.Helper()
		req := authenticatedRequest(http.MethodGet, target, "", authorization.RoleViewer)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		return res
	}
	decode := func(res *httptest.ResponseRecorder) (int, int, []applicationsecret.Metadata, map[string]any) {
		t.Helper()
		if res.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
		}
		var body struct {
			Secrets    []applicationsecret.Metadata `json:"secrets"`
			Pagination struct {
				Page       int  `json:"page"`
				PageSize   int  `json:"page_size"`
				Total      int  `json:"total"`
				TotalPages int  `json:"total_pages"`
				NextPage   *int `json:"next_page"`
			} `json:"pagination"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode %s: %v", res.Body.String(), err)
		}
		var raw map[string]any
		if err := json.Unmarshal(res.Body.Bytes(), &raw); err != nil {
			t.Fatal(err)
		}
		if _, ok := raw["pagination"]; !ok {
			t.Fatalf("response has no pagination envelope: %s", res.Body.String())
		}
		return body.Pagination.Page, body.Pagination.PageSize, body.Secrets, map[string]any{
			"total": body.Pagination.Total, "total_pages": body.Pagination.TotalPages, "next_page": body.Pagination.NextPage,
		}
	}

	// Default request: first bounded page of the default size.
	page, size, secrets, extra := decode(send("/api/v1/apps/demo/secrets"))
	if page != 1 || size != 100 || len(secrets) != 100 || secrets[0].Name != "TOKEN_000" {
		t.Fatalf("default page: page=%d size=%d items=%d first=%q", page, size, len(secrets), secrets[0].Name)
	}
	if extra["total"] != 250 || extra["total_pages"] != 3 {
		t.Fatalf("default meta: %v", extra)
	}

	// Explicit second page.
	page, size, secrets, _ = decode(send("/api/v1/apps/demo/secrets?page=2&page_size=100"))
	if page != 2 || size != 100 || len(secrets) != 100 || secrets[0].Name != "TOKEN_100" {
		t.Fatalf("page two: page=%d size=%d items=%d first=%q", page, size, len(secrets), secrets[0].Name)
	}

	// Last page ends the chain with next_page null.
	res := send("/api/v1/apps/demo/secrets?page=3&page_size=100")
	if res.Code != http.StatusOK {
		t.Fatalf("last page status %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), `"next_page":null`) || !strings.Contains(res.Body.String(), `"total_pages":3`) {
		t.Fatalf("last page envelope: %s", res.Body.String())
	}

	// Invalid parameters fail closed with the canonical error envelope.
	for _, target := range []string{
		"/api/v1/apps/demo/secrets?page=0",
		"/api/v1/apps/demo/secrets?page=abc",
		"/api/v1/apps/demo/secrets?page_size=0",
		"/api/v1/apps/demo/secrets?page_size=101",
	} {
		res := send(target)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d body %s", target, res.Code, res.Body.String())
		}
		if !strings.Contains(res.Body.String(), `"code":"bad_request"`) {
			t.Fatalf("%s: not the canonical envelope: %s", target, res.Body.String())
		}
	}

	// Metadata-only protection is preserved on paged responses.
	if strings.Contains(send("/api/v1/apps/demo/secrets?page=1&page_size=1").Body.String(), `"value"`) {
		t.Fatal("paged list response contains a value field")
	}
}
