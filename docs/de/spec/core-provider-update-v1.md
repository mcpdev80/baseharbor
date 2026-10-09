# Core-Provider-Updates – Vertrag v0.4.24

Status: **implementiert; Runtime-Abnahme echter Versionswechsel ausstehend**. Diese Seite definiert gefordertes Verhalten, keinen Nachweis eines produktiv unterstützten Komplett-Upgrades.

## Zuständigkeit und Grenzen

Das ausgewählte BaseHarbor-Release besitzt die unveränderlichen Provider-Image-Digests, Versionen, Kompatibilitätsmetadaten und den Referenzsatz für SQL, Secrets und Identity. Eine dynamische `latest`-Auflösung ist untersagt. Der unveränderliche Katalog ist in `internal/coreupdate/releases/v0.4.24.json` eingebettet; das Runtime-Inventar muss einschließlich des eigenen Keycloak-Backing-SQL zum ausgewählten Katalog passen.

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

## Explizite HA-Wiederherstellung

`baha update --recover --version VERSION --yes` stellt PostgreSQL und den etcd-DCS des ausgewählten eigenen Cores auf den verifizierten Update-Backup-Zeitpunkt zurück. Dabei wird weder ein Release installiert noch die CLI ersetzt. Transaktionen nach diesem Backup-Zeitpunkt sind nicht enthalten; ein Updatefehler löst diese Datenrücksetzung niemals automatisch aus.

Die Wiederherstellung hält den Core-Lifecycle-Lock, prüft Core-/Target-/Release-Identität und unveränderliche physische sowie DCS-Artefakte und lehnt veränderte Credentials beziehungsweise Umgebungswerte ab. PostgreSQL-Schreiber werden vor dem alten DCS gestoppt. Der neue DCS startet aus dem verifizierten isolierten Restore; die PostgreSQL-Primary wird per Stream in ein separates eigenes Volume eingespielt, Replikas werden in separaten leeren Volumes neu aufgebaut. Tatsächliche Mounts, mTLS-DCS-Identität und Quorum, SQL-Authentifizierung, Patroni-Gesundheit und Replikation müssen vor dem Commit verifiziert sein. Originalvolumes und bisheriges Manifest bleiben erhalten.

Ein Wiederanlauf nach abgeschlossenem Commit verifiziert den aktiven Cluster ohne Member-Neuerstellung oder erneute Extraktion. Mehrdeutiges Fencing, teilweise eingespielte Volumes oder ein unterbrochener Commit erfordern Abgleich; alte und neue Daten werden niemals gleichzeitig reaktiviert. Ein späteres Update verwendet ein neues Transaktionsverzeichnis, ohne die aktiven DCS-Bind-Verzeichnisse umzubenennen. Bestehende Klartext-DCS-Installationen und PostgreSQL-Major-Migrationen bleiben nicht unterstützt.

Das isolierte Acceptance-Gate prüft Replica-first-Neuerstellung und Switchover mit demselben gepinnten Image, eine ausgefallene Replika, tatsächliche PostgreSQL-WAL-/SQL-Wiederherstellung, Live-DCS-Cutover, Originalvolume-Erhalt und Wiederanlauf nach Commit. Dies zertifiziert für sich allein weder die Kompatibilität eines anderen Spilo-/PostgreSQL-Images noch eine vollständige Provider-Versionsmigration.

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

## Native Provider-Zulassung

OpenBao akzeptiert ausschließlich neuere Patches desselben Major-/Minor-Zweigs; der Kandidat verwendet 2.7.1. Keycloak folgt für HA derselben Patch-Regel. Der explizit zugelassene Single-Core-Pfad 26.7.5 → 26.8.0 stoppt und ersetzt das einzelne Mitglied und verwendet verifizierte SQL-/Konfigurationsbackups sowie Recovery. Andere Minor-/Major-Pfade werden vor jeder Änderung abgewiesen. Eine abgeschlossene native Transaktion prüft das unveränderte unveränderliche Inventar und die Secret-Scopes registrierter Anwendungen; die engere Zulassungsregel für reine Binary-Updates weist diese bereits verifizierten Anwendungen nicht erneut ab.
