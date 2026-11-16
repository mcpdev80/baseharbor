# Grundlagen: Authentifizierung und API-Fehler

BaseHarbor trennt Authentifizierung, Autorisierung und Transport.

## Authentifizierungsgrenze

Das Paket `internal/auth` definiert providerneutrale Anforderungen an OIDC und JWT. Dadurch wird der Core weder an einen bestimmten Identity-Provider noch an ein bestimmtes HTTP-Framework gekoppelt.

Aktuelle Garantien:

- OIDC-Issuer müssen HTTPS verwenden.
- Mindestens eine nicht leere Audience muss konfiguriert sein.
- Authorization-Header unterstützen ausschließlich das Schema `Bearer`.
- Fehler bei der Bearer-Verarbeitung enthalten niemals den Token-Inhalt.
- Zur JWT-Verifikation dient die kleine Schnittstelle `Verifier`.
- Jede Verifier-Implementierung muss Signatur, Issuer, Audience, Ablaufzeit und Not-before-Claims prüfen, bevor sie eine `identity.Principal` erzeugt.

Eine konkrete OIDC-Discovery- und JWKS-Implementierung wird hinter dieser Schnittstelle ergänzt; providerspezifische Konzepte dürfen nicht in den Core gelangen.

## API-Fehlergrenze

Das Paket `internal/apierror` stellt stabile, maschinenlesbare Fehlercodes und sichere Client-Meldungen bereit.

Interne Fehlerursachen können für Logs und Diagnosen eingebettet werden, werden jedoch ausdrücklich nicht als JSON serialisiert. Dadurch gelangen weder Datenbankfehler noch Secrets oder providerinterne Details über die API an Clients.

Unbekannte Fehlercodes führen zu HTTP 500 und der allgemeinen Meldung `internal error`.

## Sicherheitsprinzipien

1. Ungültige Authentifizierungskonfiguration wird abgelehnt (fail-closed).
2. Token-Inhalte erscheinen niemals in öffentlichen Fehlermeldungen.
3. Interne Ursachen werden niemals an Clients serialisiert.
4. Authentifizierung bestätigt die Identität; Autorisierung ist eine getrennte Entscheidung.
5. Besonderheiten einzelner Identity-Provider bleiben hinter Schnittstellen.
