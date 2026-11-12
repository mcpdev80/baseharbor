# Application-Befehle

Application-Befehle arbeiten mit dem portablen Contract und seinem Deployment-Zustand.

[Anwendungsbedarf erstellen und ändern](../how-to/application-needs.md) erklärt die interaktive Checkliste, Target-Provider-Präferenzen und Footprint-Vorschau.

| Befehl | Aufgabe |
| --- | --- |
| `baha new application` | Neue ecosystem-native Application erzeugen |
| `baha app new` | Interaktiven Bedarfswizard öffnen |
| `baha app edit` | Bestehenden Bedarf mit Vorschau und Zustimmung ändern |
| `baha app footprint show` | Target-bezogenen Footprint lesen |
| `baha init` | Bestehendes Repository übernehmen |
| `baha inspect` | Repository-/Application-Evidenz lesen |
| `baha app show` | Aufgelösten Vertrag anzeigen |
| `baha list` | Bekannte Applications auflisten |
| `baha plan` | Plan anzeigen |
| `baha app preflight` | Vor Mutation validieren |
| `baha up` / `baha app apply` | Application konvergieren/abgleichen |
| `baha down` | Application-Runtime stoppen |
| `baha status` | Zustand beobachten |
| `baha doctor` | Diagnostizieren, optional reparieren |
| `baha app update` | Unterstützte Application-Source aktualisieren |
| `baha destroy` | Application-eigene Ressourcen entfernen |

Auch bei einem ausgewählten Remote-Core-Target lautet der Repository-Ablauf
`baha app init`, danach `baha up`. Die Initialisierung registriert den Vertrag
und speichert die Deployment-Einstellungen beim Core. App-Name und Umgebung
kommen aus `baseharbor.yaml`; der Target-Alias legt keine Umgebung fest.
Interaktive Aufrufe zeigen die verbleibenden Änderungen aus dem Core-Plan:
Neuinstallation, Start vorhandener Dienste, Updates und offene Prüfungen.
Vorhandene Dienste stehen kompakt unter Weiterverwenden. Eine Bestätigung gibt diese Änderungen und den exakten aktuellen
App-Quellstand frei. Ohne Eingabemöglichkeit erhält der Aufrufer die
strukturierte Bestätigungsanforderung. Remote-Arbeit zeigt ihre aktuelle Phase
und Wartezeit; JSON-Ausgaben bleiben maschinenlesbar.
Beim Aktivieren von Entwicklungs-WebGUIs legt Core deren gemeinsamen Login an,
wenn noch keiner existiert. Vorhandene Zugangsdaten werden wiederverwendet.
Ein im Initialisierungsassistenten ausdrücklich gewählter Login wird einschließlich
Benutzername und eigenem oder erzeugtem Passwort auf dem ausgewählten Core
konfiguriert.
Die im Assistenten oder mit `baha dev domain DOMAIN` gewählte Entwicklungsdomain
wird auf dem ausgewählten Core konfiguriert und lokal projiziert. Neue
Entwicklungsrechner erhalten die bestehende Core-Domain als Vorschlag.
DNS-Namen müssen auf den Core zeigen; IP-Links sind davon unabhängig.
Bei gespeicherter Core-LAN-Adresse und Namen außerhalb von `.localhost`
verwendet das kanonische Gateway diese Adresse, sofern keine explizite
Bind-Einstellung vorliegt. `.localhost` bleibt auf Loopback beschränkt.
Die vorbereitete pgAdmin-Verbindung öffnet die App-Datenbank und
zeigt im Baum nur diese Datenbank. Interne Core-Datenbanken sind getrennte Ressourcen.
Ablehnung oder Abbruch der interaktiven Bestätigung stoppt den Rollout. Starte
`baha up` erneut zum Prüfen und Bestätigen; ein manueller Quellhash ist nicht nötig.
Nach erfolgreichem Remote-Deployment erscheinen eine kurze READY-Zusammenfassung
und die App-Links. IP-Links verwenden die Core-Adresse und die tatsächlich
veröffentlichten dynamischen Ports; kanonische URLs erscheinen ebenfalls.
`--json` gibt den vollständigen verifizierten Beleg aus, `--verbose` weitere
menschenlesbare Prüfergebnisse.

Für ein Remote-Target wählst du mit `baha app apply NAME --workload-port VARIABLE=PORT` einen freien veröffentlichten Workload-Port. BaseHarbor prüft, ob die Variable im Compose-Workload des Repositorys vorkommt und der Port auf dem Core-Host frei ist, und speichert die Wahl für dieses Deployment. Beispiel: `--workload-port DEMO_HTTPS_PORT=8081` weicht einem Konflikt auf Port 8080 aus, ohne die Repository-Quelle zu ändern.

## Erzeugen oder übernehmen

In einem übergeordneten Verzeichnis ohne vorhandenen Ordner `shop-api`:

```text
baha init my-app \
  --workload-source kubernetes:deploy/k8s \
  --workload-component api \
  --workload-component worker
```

Das Scaffold deklariert SQL/Cache, ergänzt Go-Clients und Runtime-Bindungsnamen. Fachliche Endpunkte und Tabellen ergänzt du selbst. Nach `baha up` zeigen `baha app logs app` die Logs und `baha app env --format json` maskierte Bindungen.

Bei vorhandenem Repository beginnst du dort mit `baha inspect .`, danach `baha init`; kein zweites Scaffold über vorhandene Dateien schreiben.

## Source-neutrale Adoption

Compose, Repository-Quadlet und Raw Kubernetes YAML werden über denselben Source-Vertrag erkannt. Das garantiert nicht ihre Runtime-Ausführung. Mehrdeutige Quellen werden nicht stillschweigend ausgewählt. `--quick` scheitert sicher, wenn keine deterministische Wahl möglich ist.

Explizite Source-/Komponentenwahl, wenn im Repository tatsächlich vorhanden:

```bash
baha new application shop-api --stack go --http --sql --cache
cd shop-api
baha inspect .
baha app show
baha plan
```

Helm und Kustomize sind noch keine Source Adapter. Eine benötigte explizite Auswahl wird in `baseharbor.repository.yaml` gespeichert.

Weitere Operationen umfassen [Backup/Restore](../how-to/backup-restore.md), Secrets, Logs, Shell/Exec, Umgebung, TLS, Evidence, Runtime-Identität und Datenzugriff. CLI und Maschinenpfade verwenden dasselbe Domänenergebnis.

Exakte Referenz: [Application-Befehle (EN)](https://mcpdev80.github.io/baseharbor/cli/applications/).


Technische Bezeichner: `baseharbor.yaml`, `baseharbor.workload-source-resolution/v1`, `baha status`, `baha plan`, `baha doctor`, `baha app ...`, `DATABASE_URL`, `DATABASE_CA_FILE`, `REDIS_URL`, `REDIS_CA_FILE`.

## Lokale Workload ohne managed Capabilities (v0.4.25)

Ein selbst geschriebenes `baseharbor.yaml` darf keine `app.id` enthalten;
`services` ist optional:

```yaml
version: 1
app:
  name: my-app
  environment: dev
```

Lege eine unterstützte, eindeutig auswählbare Compose-Workload im Repository ab
und führe `baha app init --quick`, `baha app plan --json` und `baha app apply` aus.
Init vergibt über den geschützten Repository-App-Store eine stabile ID, ohne das
autorierte YAML umzuschreiben. Lesebefehle vergeben keine ID. Fehlende oder
mehrdeutige Source ist ein Fehler und kein READY-Deployment.

Eine echte lokale Workload ohne angeforderte managed Capabilities überspringt
im vorhandenen Lifecycle Core, managed Gateway und Broker. Es braucht weder
Profil-Schalter noch implizites SQL oder Core-Login. Status und Doctor verwenden
echte Workload-Beobachtungen; Stop/Up und wiederholtes Destroy behalten die
Ownership-Prüfungen. `baha serve` erhält dadurch keine Freigabe: Seine
Management-Sicherheitsvoraussetzungen und die explizite Core-Installation
bleiben erforderlich.

Für ein bereits ausgewähltes Remote-Target genügt `baha app destroy NAME --yes`;
Target und Repository-Umgebung werden übernommen. Die CLI zeigt den aktuellen
Provider-Plan und fragt vor der Änderung nach Zustimmung. Aktivierte, verwaltete
Hosts-Einträge werden danach abgeglichen. Die Target-Gateway-CA bleibt vertraut,
solange die Core-Verbindung bestehen bleibt.

`baha app stop` (auch als `baha down` verfügbar) hält die Runtime an und behält
persistenten Zustand und Recovery-Daten. `baha app destroy NAME --yes` löscht
dagegen dauerhaft den BaseHarbor-eigenen Anwendungszustand, Zugangsdaten,
Runtime-Ressourcen, Deployment-Eintrag und Compose-Datenvolumes, deren exklusive
Anwendungszuordnung verifiziert ist. Gemeinsame, fremde, eingebundene oder nicht
eindeutig zugeordnete Daten bleiben erhalten und werden angezeigt. Gemeinsame
Target-Infrastruktur, Repository-Quelldateien und das Vertrauen des weiterhin
verbundenen Rechners in die Core-CA gehören nicht zu den Anwendungsdaten.
Provider-Folgen und Datenlöschung werden im aktuellen Plan gezeigt und müssen
ausdrücklich bestätigt werden. Ein zusätzliches `--full-reset` ist nicht nötig:
Destroy löscht die eindeutig app-eigenen Daten immer.

Plan-JSON sowie MCP/HTTP-Plan verwenden denselben `footprint`
(`baseharbor.footprint/v1`). Ein neu geplanter Binding ist `requested/unbound`
und kein Live-Nachweis. Angeforderte managed Capabilities werden mit ausgewählten
Providern und ihren echten Abhängigkeiten realisiert. Ein bedingungsloses
SQL-/Secrets-/Identity-Paket für jede App gibt es nicht. Ein unabhängig aktivierter
Management-Core behält seine Sicherheitsvoraussetzungen. Speicher-/Containerwerte
bleiben unbekannt, wenn ihre Zuordnung nicht nachgewiesen ist. Provider-Beobachtungen
bleiben bis zum geprüften nativen Inventar `unverifiable`. Nichtinteraktives
App-`--yes` autorisiert keine Einrichtung des Management-Core.

Kompatible native Placements werden wiederverwendet. Nicht unterstützte native
BYO- oder Ersatzadapter werden vor Provisionierung abgelehnt; externe OIDC-/OTLP-
Adapter bleiben unterstützt. Der Plan nennt die tatsächlichen Ausführungsgrenzen,
ohne einen funktionierenden Ersatz zu behaupten. Es gibt keine Migration alter
Manifest-Formate.

Capability-Entfernung löst das App-Binding und gleicht die verbleibenden
Target-weiten Abhängigkeiten ab. Eigene Runtime-Dienste ohne verbleibenden
Verbraucher werden entfernt. Anforderungen anderer Apps oder unabhängig aktivierter
Management-Verbraucher erhalten gemeinsam genutzte Dienste. Persistente Daten,
Credentials und Recovery-Material bleiben standardmäßig an ihren eigenen Orten.
Erhaltenes Material ermöglicht Wiederherstellung; es belegt keinen weiter
funktionierenden Dienstzugriff. Unterstütztes Wiederhinzufügen verwendet die
bestehende Systemidentität und den geschützten Provider-Zustand erneut.

Bewusste Provider-Änderungen zeigen betroffene Verbraucher, verlorene Funktionen,
gewählte Fortsetzung und erhaltenes/gelöschtes Material. Alternativlose,
destruktive oder unbekannte Folgen werden mit **Achtung** angezeigt und erfordern
zwei ausdrückliche Bestätigungen des aktuellen Plans. Gewöhnliches `--yes`
ersetzt weder diese Bestätigungen noch die Zustimmung zur Datenlöschung.
Tatsächliche Berechtigung, Ownership und Prüfung nach der Änderung bleiben
Voraussetzung. Fehlgeschlagene oder unterbrochene Ausführung bleibt teilweise
erledigt und wiederaufnehmbar. Release-Bereitschaft benötigt die jeweils erforderliche
Provider-, Mutationspfad- und native Ökosystem-Abnahme. Einzelne grüne Fixtures
belegen keine Pre-Release-Bereitschaft.


### Deploymente Identität und Wiederherstellung

Jede Anwendungs-Umgebung wird pro Target genau einmal registriert: Die
Kombination aus application_id und Umgebung identifiziert ein Deployment, und
Erstregistrierungen sind gegen gleichzeitige Registrierungen serialisiert,
damit racende `baha app init`-Läufe keine Duplikate anlegen können. Namen
dürfen sich wiederholen; die Identität ist das UUID-Paar, nicht der Name.

Hält ein Target, das vor dieser Serialisierung registriert wurde, bereits zwei
Deployments für eine Anwendungs-Identität, scheitern alle Planungskommandos auf
dem Target sichtbar und nennen beide kollidierenden Deployment-UUIDs.
`baha app list` zeigt die Zwillinge weiterhin; jedes wird per UUID stillgelegt:

```bash
baha app destroy --uuid <deployment-uuid> --yes
```

Anschließend das Target mit `baha app list` nachprüfen. Kommandos mit `--uuid`
bleiben auf einem solchen Target nutzbar; name-adressierte nicht, bis das
Duplikat stillgelegt ist. Das Paar `(application_id, environment)` ist die
System-Identität nach ADR 0017.
