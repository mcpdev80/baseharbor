package apierror

import (
	"net/http"
	"net/url"
	"strconv"
)

// Runtime list pagination contract. See docs/spec/runtime-list-pagination-v1.md.
// Every BaseHarbor runtime-facing HTTP list response carries a bounded
// pagination envelope. The contract types live in this package — the shared
// HTTP contract package every runtime API package already imports — so all
// packages serialize the metadata identically. Breaking contract changes are
// permitted before v1.0 without a compatibility shim (owner decision
// 2026-11-25).
const (
	// DefaultPageSize is the page size when the client omits page_size.
	DefaultPageSize = 100
	// MaximumPageSize is the inclusive cap for a requested page size.
	MaximumPageSize = 100
	// FirstPage is the inclusive minimum page number.
	FirstPage = 1
)

// Pagination is the pagination metadata contract object. On the wire it
// always appears as the value of a list response's `pagination` key, either
// as a named field or an embedded field in the response struct. NextPage is
// the one-based page number of the following page or nil on the last page.
// Total counts the items across all pages.
type Pagination struct {
	Page       int  `json:"page"`
	PageSize   int  `json:"page_size"`
	Total      int  `json:"total"`
	TotalPages int  `json:"total_pages"`
	NextPage   *int `json:"next_page"`
}

// PageParams is a validated pagination request.
type PageParams struct {
	Page     int
	PageSize int
}

// ParsePageParams reads the `page` and `page_size` query parameters. Absent
// parameters select the first default page. Invalid or out-of-range values
// fail closed with the canonical bad_request error; they are never clamped.
func ParsePageParams(query url.Values) (*PageParams, *Error) {
	params := &PageParams{Page: FirstPage, PageSize: DefaultPageSize}
	if raw := query.Get("page"); raw != "" {
		page, err := strconv.Atoi(raw)
		if err != nil || page < FirstPage {
			return nil, New(CodeBadRequest, "invalid page parameter", 0)
		}
		params.Page = page
	}
	if raw := query.Get("page_size"); raw != "" {
		size, err := strconv.Atoi(raw)
		if err != nil || size < FirstPage || size > MaximumPageSize {
			return nil, New(CodeBadRequest, "invalid page_size parameter", 0)
		}
		params.PageSize = size
	}
	return params, nil
}

// ParsePageRequest validates the pagination parameters of an HTTP request.
// Absent parameters select the first default page.
func ParsePageRequest(request *http.Request) (*PageParams, *Error) {
	query := url.Values{}
	if request.URL != nil {
		query = request.URL.Query()
	}
	return ParsePageParams(query)
}

// Offset is the zero-based index of the first item of the requested page.
func (p *PageParams) Offset() int {
	return (p.Page - FirstPage) * p.PageSize
}

// Paginate returns the requested page window of items and its pagination
// metadata. A page beyond the last page clamps to the last page; an empty
// collection answers page 1 with total 0. The request itself is never
// rewritten: params stays untouched.
func Paginate[T any](items []T, params *PageParams) ([]T, Pagination) {
	total := len(items)
	totalPages := (total + params.PageSize - 1) / params.PageSize
	if totalPages < FirstPage {
		totalPages = FirstPage
	}
	page := params.Page
	if page > totalPages {
		page = totalPages
	}
	start := (page - FirstPage) * params.PageSize
	end := start + params.PageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	window := items[start:end]
	var next *int
	if page < totalPages {
		following := page + 1
		next = &following
	}
	return window, Pagination{
		Page:       page,
		PageSize:   params.PageSize,
		Total:      total,
		TotalPages: totalPages,
		NextPage:   next,
	}
}
