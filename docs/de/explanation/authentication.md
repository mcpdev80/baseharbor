# Authentifizierung und Identitäten

BaseHarbor trennt Operator-Identität, Application-Benutzer und interne Provider-Zugangsdaten. Ein gemeinsamer Identity Provider macht daraus keine gemeinsame Berechtigungsgrenze.

## Operator-Zugriff

Unkonfiguriertes `dev` kann im trusted-local-Modus ohne Login arbeiten. Nach expliziter Erst-Owner-Initialisierung verwendet es die gespeicherte Operator-OIDC-Grenze. Geschützter Zugriff in `test` und `prod` benötigt eine gültige OIDC-Operator-Sitzung für Target und Umgebung. Die CLI verwendet Authorization Code mit PKCE und kurzlebigen, lokal geschützten Sitzungszustand.

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

## Vertrauenswürdigkeit der Managed-CA ohne OS-Eingriffe

Der Managed-Operator-Issuer wird von Keycloak hinter der installationseigenen Managed-CA
(„BaseHarbor Managed Service CA") bedient, die im Trust-Pool des Betriebssystems nicht
installiert ist. Die CLI verlässt sich daher nicht allein auf den OS-Trust-Pool: Sie vertraut
die Managed-CA, die sie beim Core-Bootstrap erstellt hat, zusätzlich zum System-Pool an (additiv;
der System-Pool wird niemals ersetzt).

Die Vertrauenswürdigkeit wird in dieser Reihenfolge aufgelöst:

1. `--ca FILE` auf dem Login-Befehl.
2. `BASEHARBOR_OPERATOR_OIDC_CA_FILE` (zustandsübergreifender Alias).
3. Die Standarddatei im Zustandsverzeichnis `trust/managed-operator-ca.pem`, die beim Core-Bootstrap geschrieben wird.

Löst keine dieser Optionen ein Zertifikat auf, schläf der Discovery fehl und nennt den Korrekturpfad
(Installiere die Managed-CA oderweise mit `--ca` bzw. `BASEHARBOR_OPERATOR_OIDC_CA_FILE` darauf).

## Headless PKCE auf Hosts ohne Browser

Auf SSH-/PTY-less-Hosts erfordert der Operator-Login keinen echten Browser. Der Loopback-Callback
bindet weiter (fester Port bei `BASEHARBOR_OPERATOR_OIDC_CALLBACK_PORT`), und die CLI gibt die
Autorisierungs-URL aus; der Autorisierungscode wird headless abgeschlossen:

```sh
# 1. Start the login; it prints the authorization URL and waits for the code.
baha login -e dev --init-tenant --auth-code-file /tmp/baha-auth-code --no-browser

# 2. In another shell, open the printed URL in a real browser, authenticate, and copy the
#    returned authorization code into the file.
printf 'MjM0UTflfn...authorization-code...' > /tmp/baha-auth-code
```

`BASEHARBOR_OPERATOR_OIDC_AUTH_CODE_FILE` ist der zustandsübergreifende Alias von `--auth-code-file`,
und `BASEHARBOR_NO_BROWSER=1` ist der Alias von `--no-browser`. Der Callback-Port wird mit
`BASEHARBOR_OPERATOR_OIDC_CALLBACK_PORT` Ende-Ende eingehalten, sodass die ausgegebene Loopback-URL
dirett ansteuerbar ist. Dieselbe headless-Grenze gilt für `baha node add`, das die Operator-Sitzung
dieselbe abgeschlossen hat, bevor es einen Remote-Node enrollt.

## Erster Operator und Tenant

Der explizite Befehl `baha login -e dev --init-tenant` initialisiert nach verifiziertem PKCE-Login den Tenant `default` und die `editor`-Membership des ersten Operators. Er setzt lokalen Core-Besitz, Core-Readiness, gültige OIDC-Identität und eine geschützte PostgreSQL-TLS-Verbindung voraus; Wiederholung durch denselben Owner bleibt idempotent. Fremde Übernahmen und Remote-Targets werden abgewiesen. Die [vollständige Einrichtung einschließlich Connector und Issuer/Audience-Abgleich](../cli/security-trust.md) ist gesondert dokumentiert. Umsetzung in Core-PR #1437, native Abnahme offen.

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
