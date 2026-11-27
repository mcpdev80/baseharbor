# Runtime List Pagination v1

## Scope

This specification defines the pagination contract for BaseHarbor runtime-facing
HTTP list responses. It applies to every JSON collection a client can list on the
Core listeners: the application secret list, the application runtime broker
capabilities list, the source-transfer pending delta list and the machine
navigation list results (`app.list`, `target.list`, `workspace.list`,
`runtime.list`).

Before v1.0 this is a breaking contract change without a compatibility shim
(owner decision 2026-11-25). The shared contract types live in
`internal/apierror` (`Pagination`, `PageParams`); the
navigation helpers live in `internal/machine/navigation.go`.

## Request parameters

| Parameter | Meaning | Default | Bounds |
| --- | --- | --- | --- |
| `page` | one-based page number | `1` | integer ≥ 1 |
| `page_size` | maximum items per page | `100` | integer 1…100 |

A client MUST send the parameters as query parameters of the list request. An
invalid or out-of-range value MUST fail closed with the canonical
`apierror` envelope `{"code":"bad_request", …}` and HTTP 400; servers MUST NOT
clamp a malformed request. A page beyond the last page answers the last page
(it never answers an empty page with a non-empty total).

## Response envelope

Every list response carries a `pagination` object alongside its item
collection:

```json
{
  "secrets": [ … ],
  "pagination": {
    "page": 1,
    "page_size": 100,
    "total": 250,
    "total_pages": 3,
    "next_page": 2
  }
}
```

- `page` / `page_size`: the answered window.
- `total`: the item count across all pages of the complete Core observation.
- `total_pages`: `ceil(total / page_size)`, at least 1.
- `next_page`: the following page number, or `null` on the last page.

Machine navigation results (`app.list`, `target.list`, `workspace.list`,
`runtime.list`) embed the same envelope on the result record. A result produced
without a page request — the CLI JSON path and MCP — reports the complete Core
observation as one bounded page: `page` 1 with `page_size` equal to the observed
`total`. An empty collection answers `total` 0 with a single empty page.

## Consumer rules

- A consumer follows `next_page` until it is `null`. A consumer MUST NOT
  assume a list is complete after the first page when `next_page` is present.
- Paging never reorders items within `total`; ordering is the domain
  responsibility documented by each list's own contract.
- The envelope carries no secret values, tokens or provider credentials, and
  the `Cache-Control: no-store` semantics of the underlying endpoints are
  unchanged.

## Authoritative inventory (contract state 2026-11-28)

| Endpoint / result | Items | Pagination |
| --- | --- | --- |
| `GET /api/v1/apps/{app}/secrets` | `secrets` | paged, `page`/`page_size` query parameters |
| `GET /runtime/v1/capabilities` | `capabilities` | paged, `page`/`page_size` query parameters |
| `GET /api/v1/machine/source-transfers` | `missing` | paged, `page`/`page_size` query parameters |
| `app.list` / `target.list` / `workspace.list` / `runtime.list` | result collections | envelope always present; windowed when the HTTP execution request carries `page`/`page_size` |
