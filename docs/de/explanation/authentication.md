# Authentifizierung und Identitäten

BaseHarbor trennt Operator-Identität, Application-Benutzer und interne Provider-Zugangsdaten. Ein gemeinsamer Identity Provider macht daraus keine gemeinsame Berechtigungsgrenze.

## Operator-Zugriff

Lokales `dev` kann im trusted-local-Modus ohne Login arbeiten. Geschützter Zugriff in `test` und `prod` benötigt eine gültige OIDC-Operator-Sitzung für Target und Umgebung. Die CLI verwendet Authorization Code mit PKCE und kurzlebigen, lokal geschützten Sitzungszustand.

```text
Authorization: Bearer <ID token>
        ↓
OIDC discovery + JWKS verification
        ↓
identity.Principal { issuer, subject, audience }
        ↓
identity-scoped membership resolution
        ↓
tenancy.Context { tenant, external identity, roles }
        ↓
RBAC + application ownership + protected handler
```

Managed Keycloak oder externes OIDC können diese Grenze bereitstellen. Application- und Operator-Clients, Scopes und logische Identitätsbereiche bleiben getrennt. Login erweitert keine Policy-Berechtigungen.

## Verifikation und Tenant

Geschützte Anfragen prüfen OIDC-Signatur, Issuer, Ablauf und akzeptierte Audience. Bearer-Tokens gehören nicht in normale Fehler, Antworten, Logs oder persistierten Identitätszustand. Vor RBAC und Application-Besitzprüfung wird die externe Identität genau einem Tenant zugeordnet. Fehlende oder mehrdeutige Memberships scheitern sicher.

Die Vorabauflösung verwendet eine identitätsgebundene SELECT-RLS-Policy, weder Superuser noch `BYPASSRLS`. Danach gilt Tenant-RLS.

## Application-Identität

Workloads verwenden ihre normale OIDC/OAuth2-Bibliothek. Managed Identity stellt einen isolierten Application-/Environment-Bereich und Client bereit; Redirect-/Logout-URIs folgen der Exposure. Externes OIDC kann Authentifizierung ohne Provisionierung liefern.

Standardbindungen sind `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_SCOPES`, optional `OIDC_CLIENT_SECRET_FILE` und `OIDC_CA_FILE`. Privates Vertrauensmaterial wird als Datei projiziert, im Service Binding auch als `ca.crt`. Provider-Admin-Zugangsdaten gelangen nicht in Workloads.

## Lokale Management-Oberflächen

Ein Development-Target besitzt ein separates Management-Konto, standardmäßig `developer`, mit starkem generiertem Passwort. Managed OIDC kann diese lokale Identität zentralisieren; andere Provider-UIs verwenden native Adapter. `test` und `prod` verwenden diese gemeinsame Dev-Zugangsdaten nicht.

Nur `baha dev credentials` zeigt Zugangsdaten ausdrücklich an. Status, Doctor, Plan und Evidence enthalten das Passwort nicht. Die Target-Domain ist standardmäßig `baha.localhost`; Browser- und interne Workload-Endpunkte werden passend zum jeweiligen Netz realisiert.

Weiter: [Security-/Trust-Befehle](../cli/security-trust.md), [Sicherheit](security.md), [technische Erklärung (EN)](https://mcpdev80.github.io/baseharbor/explanation/authentication/).


## Weitere unveränderte technische Beispiele

```text
baseharbor.identity_issuer
baseharbor.identity_subject
```

```text
pre-tenant request phase
    verified issuer + subject
    -> SELECT-only identity policy

post-resolution request phase
    resolved tenant_id
    -> normal tenant RLS policy
```


Technische Kennungen: `internal/auth.OIDCVerifier`, `github.com/coreos/go-oidc/v3/oidc`, `https://auth.<domain>`, `https://auth-admin.<domain>`, `baha login -e ENV`, `baha whoami -e ENV`, `baha logout -e ENV`, `baseharbor.tenant_id`, `0004_identity_membership_resolution`, `memberships`, `FOR SELECT`, `database.IdentityTenantResolver`, `set_config`, `tenancy.Resolve`, `ErrNoMembership`, `ErrAmbiguousTenant`.
