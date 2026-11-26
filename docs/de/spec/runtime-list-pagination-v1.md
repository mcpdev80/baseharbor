# Runtime-Listen-Seitennummerierung v1

Diese Seite ist die deutsche Paritätsreferenz zum kanonischen englischen
Vertragstext: [Runtime List Pagination v1 (EN)](../../spec/runtime-list-pagination-v1.md).
Der englische Text bleibt die normative Autorität.

Geltungsbereich: every JSON collection a client can list on the Core listeners:
the application secret list, the application runtime broker capabilities list,
the source-transfer pending delta list and the machine navigation list results
(`app.list`, `target.list`, `workspace.list`, `runtime.list`). Vor v1.0 ist
dies eine bewusste Vertragsänderung ohne Kompatibilitäts-Übergang (owner
decision 2026-11-25). Die geteilten Vertragstypen liegen in `internal/apierror`
(`Pagination`, `PageParams`); die Navigations-Helfer in
`internal/machine/navigation.go`.

Request-Parameter: `page` (one-based) und `page_size` (Maximum 100) als
Query-Parameter. A malformed or out-of-range value MUST fail closed mit dem
kanonischen `apierror`-Umschlag `{"code":"bad_request", …}` und HTTP 400;
servers MUST NOT clamp a malformed request.

Jede Listen-Antwort trägt das `pagination`-Objekt mit `page`, `page_size`,
`total`, `total_pages` und `next_page` (`null` auf der letzten Seite) neben der
Item-Sammlung, zum Beispiel für `secrets`, `capabilities` oder `missing`:

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

Das Ende der Kette folgt die Regel `ceil(total / page_size)` für
`total_pages`. Ein Consumer follows `next_page` until it is `null`.

Endpunkt-Inventar (contract state 2026-11-28):

| Endpoint / result | Items | Pagination |
| --- | --- | --- |
| `GET /api/v1/apps/{app}/secrets` | `secrets` | paged, `page`/`page_size` query parameters |
| `GET /runtime/v1/capabilities` | `capabilities` | paged, `page`/`page_size` query parameters |
| `GET /api/v1/machine/source-transfers` | `missing` | paged, `page`/`page_size` query parameters |
| `app.list` / `target.list` / `workspace.list` / `runtime.list` | result collections | envelope always present; windowed when the HTTP execution request carries `page`/`page_size` |

Die Semantik `Cache-Control: no-store` der zugrunde liegenden Endpunkte bleibt
unverändert; der Umschlag enthält niemals secret values, tokens oder provider
credentials. Die vollständige, normative Fassung liegt in der englischen
Quelle.
