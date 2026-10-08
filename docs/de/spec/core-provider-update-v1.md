# Core-Provider-Updates – Vertrag v0.4.24

Status: **Implementierung in Arbeit**. Diese Seite definiert gefordertes Verhalten, keinen Nachweis eines produktiv unterstützten Komplett-Upgrades.

## Zuständigkeit und Grenzen

Das ausgewählte BaseHarbor-Release besitzt die unveränderlichen Provider-Image-Digests, Versionen, Kompatibilitätsmetadaten und den Referenzsatz für SQL, Secrets und Identity. Eine dynamische `latest`-Auflösung ist untersagt. Aktuelle Pin-Konstanten sind noch kein versioniertes Release-Manifest.

Die Domäne `internal/coreupdate` arbeitet auf **eigenen Realisierungen**. Installation, Placement-Scope, Instanz und Eigentümer bestimmen die Identität. Fremde beziehungsweise externe Provider dürfen nicht automatisch geändert werden.

Der Planer unterscheidet:

- `no_change`: Version, Image und Digest unverändert.
- `safe_reconcile`: geprüfter kompatibler zustandsloser Pfad.
- `backup_recovery_required`: unterstützter datenhaltiger Pfad mit verifiziertem Wiederherstellungspunkt.
- `migration_required`: unterstützte Schema-/Datenmigration mit expliziter Verifikation und Recovery.
- `unsupported`: vor Änderungen ablehnen und den Grund nennen.

Ein PostgreSQL-Major-Upgrade bleibt für den allgemeinen Planer nicht unterstützt. Es braucht einen eigenen geprüften Migrationspfad; bloßer Image-Austausch beweist kein Daten-Upgrade.

## Ablauf

1. Exaktes Release und erwartete Digests auflösen.
2. Eigene Shared- und isolierte Realisierungen prüfen, externe ohne Übernahme erkennen.
3. Sämtliche Änderungen vorab planen und vorprüfen.
4. Für datenhaltige Upgrades verifizierte Recovery-Points verlangen.
5. Den vorhandenen provider-nativen Core-Lifecycle ausführen und Evidence sichern.
6. SQL, Secrets und Identity tatsächlich auf Verwendbarkeit prüfen, nicht nur Container-Readiness.
7. Unvollständige eigene Zustände idempotent fortsetzen. Kein Rollback-Versprechen für irreversible Datenmigrationen.
8. Erfolg erst nach Verifikation aller betroffenen Realisierungen melden.

## Human- und Machine-Parität

`baha update --check`, CLI-Ausgabe, JSON, MCP und geschütztes HTTP müssen dieselbe Plan-/Ergebnisdomäne abbilden. Das bisherige Self-Update für Binary-/Release-Artefakte darf ohne Provider-Update-Integration kein vollständiges Core-Upgrade behaupten.

## Für ein Release erforderliche Evidence

- Unveränderlicher Provider-Satz und Ermittlung installierter Versionen.
- Kein Neustart bei unverändertem Zustand.
- Kompatibles PostgreSQL-Upgrade und Ablehnung nicht unterstützter Major-Upgrades.
- Erhalt von OpenBao-Seal-, Unseal- und Recovery-Zustand.
- Keycloak-Authentifizierung und Issuer-Verifikation nach Migration.
- Shared genau einmal, isolierte Realisierungen separat.
- Fremde Provider bleiben unverändert.
- Teilfehler mit Wiederanlauf ohne duplizierten Zustand.
- Docker rootless und Podman rootless als tatsächliche Runtime-Nachweise.
- Evidence an Release-SHA und Provider-Digests gebunden.

Anfängliche Planer-/Hook-Tests beweisen nur den Vertrag, **nicht** die realen Provider-Mutationspfade.
