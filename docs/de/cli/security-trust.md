# Security und Trust

## Host-Vertrauen

Nach erfolgreichem lokalem Deployment:

```text
baha trust status
baha trust export --output PATH
baha trust install --yes
```

Exportiert wird die öffentliche managed-local CA, kein privater Schlüssel. Installation ins Host-Vertrauen ist eine ausdrücklich genehmigte Host-Mutation mit `baha trust install --yes`. Externe Firmen-/BYOC-CAs bleiben Operator-Besitz. Danach den Status erneut prüfen.

## OpenBao

```text
baha openbao status
baha openbao bootstrap --recovery-file PATH
baha openbao unseal --recovery-file PATH
baha openbao rotate --recovery-file PATH
```

Rotation ersetzt Manager-AppRole, Control-Plane-Datenbankzugangsdaten und managed Service-PKI. Neue Pfade werden geprüft, bevor altes Material zurückgezogen wird. Recovery-Material bleibt geschützt beim Operator/Core, nicht im Browser-Eingabevertrag.

## Operator und Verbindungen

`baha login`, `baha whoami` und `baha logout` verwalten die Operator-Sitzung. Lokales Development kann trusted-local bleiben; test/prod folgen der konfigurierten OIDC-Grenze. Tokens nicht in Befehlsargumente kopieren.

`baha connect`, `baha disconnect` und `baha connections` verwalten explizite Verbindungen. Zugangsdaten, private Schlüssel und Secret-Werte gehören nicht in Application Intent oder normale Maschinenausgabe.

Weiter: [Sicherheit](../explanation/security.md), [Authentifizierung](../explanation/authentication.md), [exakte Befehle (EN)](https://mcpdev80.github.io/baseharbor/cli/security-trust/).


## Initialen Operator und Tenant einrichten (v0.4.27, Kandidat)

Auf dem **lokal gehörenden, bereiten Management-Core** initialisiert der erste Owner ausdrücklich:

```bash
baha target show
baha up --control-plane-only
baha login -e dev --init-tenant
baha whoami -e dev
```

Diese Erst-Owner-Aktion ist **kein automatischer Bestandteil von `baha up`**. Die Anmeldung verwendet interaktives OIDC Authorization Code + PKCE. Ohne gespeicherte Operator-OIDC-Grenze lässt sich im Einrichtungsdialog Managed/Shared Keycloak oder ein externer OIDC-Provider wählen. Managed Keycloak nutzt ein eigenes umgebungsgebundenes Operator-Realm samt Client, getrennt vom Core-Identity-Realm `baseharbor`.

Erst nach verifizierter, nicht abgelaufener Anmeldung werden Tenant `default`, die externe Identität (Issuer + Subject) und deren `editor`-Membership atomar mit RLS gespeichert. Derselbe Owner kann wiederholen; andere Owner, fremde bestehende Tenants, Remote-Targets, ungültige Identitäten oder ein nicht bereiter Core scheitern sicher. Es gibt keinen anonymen Erst-Admin-Endpunkt. Unkonfiguriertes `dev` bleibt trusted-local; nach dem expliziten Bootstrap nutzt normales `baha login -e dev` die gespeicherte OIDC-Grenze.

**Vor der Connector-Einschreibung:** `BASEHARBOR_API_OIDC_ISSUER` und `BASEHARBOR_API_OIDC_AUDIENCES` der geschützten Operator-API müssen zum tatsächlichen Operator-Realm/Client passen. Eine API mit `.../realms/baseharbor` akzeptiert keine Token aus `.../realms/bh-operators-dev`. Issuer und Audience über Discovery abgleichen, keine OIDC-/Tenant-Prüfung umgehen.

Anschließend mit `baha node add` einschreiben, mit `baha node connect` verbinden, über `baha app up` bereitstellen und mit `baha doctor` prüfen. Docker und Podman brauchen jeweils echte End-to-End-Nachweise. Die Funktion entsteht in [Core-PR #1437](https://github.com/mcpdev80/baseharbor/pull/1437) und ist **noch weder gemergt noch released oder nativ abgenommen**.

## Weitere Befehlsbeispiele

```text
baha login
baha logout
baha whoami
```

```text
baha connect
baha disconnect
baha connections
```

```bash
baha trust status
baha trust export --output ./baseharbor-dev-ca.pem
```

```bash
baha trust install --yes
```

```bash
baha whoami
baha login
baha whoami
baha logout
```
