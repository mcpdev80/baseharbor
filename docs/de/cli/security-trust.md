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


## Workstation-Gerät registrieren

Die Verbindung eines Entwicklungsrechners mit seinem Core ist eine separate
Geräte-Einschreibung, keine OIDC-Anmeldung eines Operators und keine
Node-Connector-Einschreibung. Auf der Core-VM `baha up` und danach
`baha onboard` ausführen. Die private JSON-Einladung auf die Workstation
übertragen, dort mit `baha onboard --import FILE` importieren und das gemeldete
Target mit `baha use TARGET` auswählen. Die Einladung registriert eine
Geräteidentität mit minimalen Rechten, keine Operator-Rolle. Siehe den
[Ablauf Core-zu-Workstation](../spec/zero-friction-workstation-onboarding-v0427.md).

Menschliche Operatoren melden sich weiterhin über die konfigurierte
OIDC-Grenze mit `baha login` an. Die Geräteeinschreibung verändert oder
umgeht diese Authentifizierungsrichtlinie nicht.

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
