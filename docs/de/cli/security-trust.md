# Security und Trust

## Host-Vertrauen

Nach erfolgreichem lokalem Deployment:

```bash
baha trust status
baha trust export --output ./baseharbor-dev-ca.pem
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
