# Application sichern und wiederherstellen

Im deployten Application-Repository erzeugt Backup eine verschlüsselte Recovery-Einheit für eigene Daten.

```bash
baha app backup --output ../orders-api.baha-backup
```

Der Terminal-Flow fragt verdeckt nach einem Passwort und zeigt den Recovery-Scope. Passwort getrennt vom Archiv bewahren. Eine vorhandene Archivdatei allein beweist keine erfolgreiche oder wiederherstellbare Sicherung.

## Automation

Eine owner-only Passwortdatei außerhalb von Git durch deinen Secret Manager vorbereiten:

```bash
baha --no-input app backup --password-file "$HOME/.config/orders/backup-password" --output ../orders-api.baha-backup
```

Archiv und erforderliches Recovery-Material in geschützten Off-Host-Speicher kopieren. Der Application-Name ersetzt kein verlorenes Passwort.

## Bewusst wiederherstellen

Restore verändert den ausgewählten eigenen Recovery-Zustand. Erst Ziel und Application prüfen:

```bash
baha target show
baha app show
```

Nach Prüfung:

```bash
baha app restore ../orders-api.baha-backup --password-file "$HOME/.config/orders/backup-password"
baha status
baha doctor
```

BaseHarbor validiert und entschlüsselt vor Mutation, stellt eigene Zustände wieder her, bindet neu und verifiziert Capabilities. Danach einen bekannten Geschäftsdatensatz über die eigene API/Datenbank prüfen.

## Shared-SQL-Grenze

SQL-Instanzen `default` und `analytics` der ausgewählten Application gehören in ihren Scope, Geschwister-Datenbanken und provider-globaler Zustand nicht. Es wird kein `pg_dumpall` verwendet. Restore löscht keine Geschwister-Datenbanken und verändert deren Rollen/Zugangsdaten nicht. Besitz wird vor und nach Mutation geprüft.

[Exakte Recovery-Referenz (EN)](https://mcpdev80.github.io/baseharbor/reference/backup-and-restore/).
