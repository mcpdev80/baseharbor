package apierror

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

func query(t *testing.T, raw string) url.Values {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Query()
}

func TestPageParamsDefaults(t *testing.T) {
	params, err := ParsePageParams(query(t, "/x"))
	if err != nil {
		t.Fatal(err)
	}
	if params.Page != FirstPage || params.PageSize != DefaultPageSize || params.Offset() != 0 {
		t.Fatalf("unexpected defaults: %+v", params)
	}
}

func TestPageParamsRejectsInvalidValues(t *testing.T) {
	for _, raw := range []string{
		"/x?page=0", "/x?page=-1", "/x?page=abc", "/x?page=1.5",
		"/x?page_size=0", "/x?page_size=-3", "/x?page_size=abc",
		"/x?page_size=101", "/x?page_size=9999999999999999999999",
	} {
		if _, err := ParsePageParams(query(t, raw)); err == nil {
			t.Fatalf("%s must fail closed", raw)
		} else if err.Code != CodeBadRequest || err.Status != 400 {
			t.Fatalf("%s: unexpected error %#v", raw, err)
		}
	}
}

func TestPageParamsAcceptsBoundaryValues(t *testing.T) {
	params, err := ParsePageParams(query(t, "/x?page=2&page_size=100"))
	if err != nil {
		t.Fatal(err)
	}
	if params.Page != 2 || params.PageSize != MaximumPageSize || params.Offset() != 100 {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestParsePageRequest(t *testing.T) {
	params, err := ParsePageRequest(&http.Request{URL: mustParse(t, "/x?page_size=25")})
	if err != nil || params.Page != FirstPage || params.PageSize != 25 {
		t.Fatalf("params=%+v err=%v", params, err)
	}
	if _, err := ParsePageRequest(&http.Request{URL: mustParse(t, "/x?page=zero")}); err == nil {
		t.Fatal("invalid page must fail closed")
	}
}

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestPaginateWindowsAndLastPage(t *testing.T) {
	items := make([]int, 250)
	for i := range items {
		items[i] = i
	}
	window, meta := Paginate(items, &PageParams{Page: 2, PageSize: 100})
	if len(window) != 100 || window[0] != 100 {
		t.Fatalf("unexpected window: %d %v", len(window), window)
	}
	if meta.Total != 250 || meta.TotalPages != 3 || meta.Page != 2 {
		t.Fatalf("unexpected meta: %+v", meta)
	}
	if meta.NextPage == nil || *meta.NextPage != 3 {
		t.Fatalf("expected next_page 3, got %v", meta.NextPage)
	}
	_, last := Paginate(items, &PageParams{Page: 3, PageSize: 100})
	if last.NextPage != nil || last.TotalPages != 3 {
		t.Fatalf("last page must end the chain: %+v", last)
	}
	_, beyond := Paginate(items, &PageParams{Page: 9, PageSize: 100})
	if beyond.Page != 3 || beyond.NextPage != nil {
		t.Fatalf("beyond-last clamps to last page: %+v", beyond)
	}
	empty, meta := Paginate([]int(nil), &PageParams{Page: 1, PageSize: DefaultPageSize})
	if len(empty) != 0 || meta.Total != 0 || meta.TotalPages != 1 || meta.NextPage != nil {
		t.Fatalf("empty collection: %+v", meta)
	}
}

func TestPaginationEnvelopeJSONShape(t *testing.T) {
	items := []string{"a", "b"}
	window, meta := Paginate(items, &PageParams{Page: 1, PageSize: 2})
	raw, err := json.Marshal(struct {
		Items      []string   `json:"items"`
		Pagination Pagination `json:"pagination"`
	}{Items: window, Pagination: meta})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	envelope, ok := decoded["pagination"].(map[string]any)
	if !ok {
		t.Fatalf("envelope missing in %s", raw)
	}
	if envelope["page"] != float64(1) || envelope["page_size"] != float64(2) || envelope["total"] != float64(2) || envelope["total_pages"] != float64(1) {
		t.Fatalf("unexpected envelope: %v", envelope)
	}
	if envelope["next_page"] != nil {
		t.Fatalf("last page must answer next_page null, got %v", envelope["next_page"])
	}
}
