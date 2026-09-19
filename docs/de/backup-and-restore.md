# Backup und Restore

BaseHarbor behandelt Backup als eine verschluesselte Recovery-Einheit pro Anwendung statt als lose Sammlung einzelner Dumps. Backup gilt erst zusammen mit ausgeuebtem Restore und erfolgreicher Post-Verifikation als unterstuetzt.

## Backup erstellen

Interaktiv:

```bash
baha app backup
```

Vor der Mutation zeigt BaseHarbor Anwendung/Environment, Zielarchiv, dauerhafte PostgreSQL-Ressourcen, Managed-Secret-Einschluss und den temporaeren Snapshot-Impact. Die Passwort-Eingabe deaktiviert Terminal-Echo, verlangt Bestaetigung und legt das Passwort nie in argv. Zu kurze Passwoerter oder abweichende Bestaetigung koennen interaktiv erneut eingegeben werden.

Fuer Automation bleibt der deterministische Pfad erhalten:

```bash
baha app backup --password-file ./backup-password.txt
```

Mit explizitem Ziel:

```bash
baha app backup \
  --password-file ./backup-password.txt \
  --output ./mailflow-production.bhbackup
```

Vor der Sicherung verifiziert BaseHarbor die Runtime und den OpenBao-Scope, stoppt bei Bedarf Workload und Secret Broker kontrolliert, erfasst den Zustand, schreibt das verschluesselte Archiv und startet die zuvor gestoppten Komponenten wieder.

Die Recovery-Einheit umfasst:

- deklarative Anwendungsmetadaten;
- alle von BaseHarbor verwalteten PostgreSQL-Instanzen;
- den anwendungseigenen OpenBao-Secret-Scope, sofern aktiviert.

Nach erfolgreichem Backup wird nicht-geheimer Metadatenzustand wie Archivpfad, Erstellungszeit und enthaltene logische Ressourcen geschuetzt gespeichert und kann ueber `baha app show` angezeigt werden.

## Restore

Interaktiv:

```bash
baha app restore ./mailflow-production.bhbackup
```

Das Archiv wird vor destruktiver Mutation entschluesselt und validiert. Anschliessend zeigt BaseHarbor Anwendung/Environment, Archivzeitpunkt, enthaltene Ressourcen und Restore-Impact. Guided Restore verwendet standardmaessig **No** und mutiert erst nach expliziter Bestaetigung.

Fuer Automation:

```bash
baha app restore ./mailflow-production.bhbackup \
  --password-file ./backup-password.txt
```

Danach werden BaseHarbor-eigene Zielressourcen kontrolliert neu aufgebaut, PostgreSQL und Secrets bei gestopptem Workload wiederhergestellt, Runtime-Identitaeten regeneriert und die gestartete Application Boundary erneut verifiziert.

Nach Restore erhaelt ein Repository-Workload ein begrenztes Readiness-Fenster, um Service-Health und HTTP/TLS-Exposure zu erreichen. Das lockert fail-closed nicht: Erfolg wird nicht gemeldet, nur weil Container gestartet sind. Wird die verifizierte Grenze innerhalb des verantworteten Timeouts nicht READY, schlaegt Restore fehl.

`Status: READY` wird erst nach erfolgreicher Verifikation von Backend-State, Managed Secrets, regenerierter Runtime Identity, Workload und anwendbaren HTTP/TLS-Checks ausgegeben. Erst dann wird erfolgreiche Recovery-Metadatenablage geschrieben.

Falsches Passwort, defektes Archiv, Identitaetskonflikte, fehlgeschlagene Preflights oder fehlgeschlagene Abschlussverifikation werden niemals zu einem erfolgreichen Restore.

## Progress-Ausgabe

Guided Backup und Restore zeigen sofort eine sichtbare Aktivitaetsanzeige. Sie ist absichtlich indeterminiert und erfindet keine Prozentwerte. Die zugrunde liegende Befehlsausgabe bleibt gepuffert verfuegbar, damit Fehler beobachtbar und actionable bleiben.

Nicht-interaktive `--password-file`-Automation haengt nicht von Terminal-Rendering ab.

## Scope

Object-Storage-Inhalte sind noch nicht Bestandteil der BaseHarbor-Recovery-Einheit. Seit v0.4.6 brechen `baha app backup` und `baha app restore` deshalb fuer Anwendungen mit Managed `object-storage.s3` fail-closed ab, statt ein unvollstaendiges Archiv als recoverbar darzustellen. Anwendungseigene Dateien, externe Datenbanken und weiterer externer Zustand benoetigen weiterhin einen eigenen Backup-/Recovery-Mechanismus.

Archivformat, Kryptographie und Restore-Semantik gehoeren zum Release-Kompatibilitaetsvertrag.
