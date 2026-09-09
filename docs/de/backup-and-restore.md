# Backup und Restore

BaseHarbor behandelt Backup als eine verschlüsselte Recovery-Einheit pro Anwendung statt als lose Sammlung einzelner Dumps.

## Backup erstellen

```bash
baha app backup --password-file ./backup-password.txt
```

Mit explizitem Ziel:

```bash
baha app backup \
  --password-file ./backup-password.txt \
  --output ./mailflow-production.bhbackup
```

Vor der Sicherung verifiziert BaseHarbor die Runtime und den OpenBao-Scope, stoppt bei Bedarf Workload und Secret Broker kontrolliert, erfasst den Zustand, schreibt das verschlüsselte Archiv und startet die zuvor gestoppten Komponenten wieder.

Die Recovery-Einheit umfasst:

- deklarative Anwendungsmetadaten;
- alle von BaseHarbor verwalteten PostgreSQL-Instanzen;
- den anwendungseigenen OpenBao-Secret-Scope, sofern aktiviert.

## Restore

```bash
baha app restore ./mailflow-production.bhbackup \
  --password-file ./backup-password.txt
```

Das Archiv wird vor der ersten Mutation vollständig validiert und entschlüsselt. Danach werden nur BaseHarbor-eigene Zielressourcen kontrolliert neu aufgebaut, PostgreSQL und Secrets bei gestopptem Workload wiederhergestellt, Runtime-Identitäten regeneriert und der gestartete Anwendungsrand erneut verifiziert.

Falsches Passwort, defektes Archiv, Identitätskonflikte oder fehlgeschlagene Preflights stoppen den Restore fail-closed.

Anwendungseigene Dateien außerhalb von BaseHarbor-verwaltetem PostgreSQL/OpenBao sind nicht Bestandteil dieser Recovery-Einheit.