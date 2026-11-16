# Veröffentlichungen und Versionierung

[Pre-Release-Bereitschaft](../development/pre-release-readiness.md)
BaseHarbor verwendet Semantic Versioning für normale Versionen mit Git-Tags, die von `v`. Während der v0.4 Linie verwenden Notfall-Hotfixes den dokumentierten Vier-Teil ` MAJOR.MINOR.PATCH.HOTFIX`Erweiterung.

Beispiele:`v0.3.0 `, ` v0.4.15 `, ` v0.4.15.1 `, ` v1.0.0`.

## Stabilitätspolitik

BaseHarbor ist derzeit in der `0.x` Reihe.

Während `0.x`:

- Patch-Freisetzungen (`0.4.0 ` -> ` 0.4.1`) innerhalb der aktiven Nebenlinie rückwärtskompatibel bleiben und Korrekturen, Sicherheitsverhärtung und additive Fähigkeiten enthalten können;
- geringfügige Freisetzungen (`0.4.x ` -> ` 0.5.0`) können absichtlich dokumentierte Bruchänderungen oder größere Vertragsverlagerungen enthalten;
- jede Bruchänderung muss ausgerufen werden `CHANGELOG.md` und GitHub Release-Notizen;
- Verbraucher sollten einen expliziten kompatiblen Bereich statt Tracking pin `main`.

Nach `v1.0.0`, öffentliche CLI zu brechen, manifestieren, persistent-state oder unterstützt Integrationsänderungen erfordern eine neue Hauptversion.

## Vertrag über die Vereinbarkeit

Der versionierte Vertrag enthält dokumentierte `baha` Befehle und Flags, die `baseharbor.yaml` Schema, fortbestehender BaseHarbor-Status, unterstützte Backup/Restore-Formate, anwendungsorientierte Umgebungs-/Serviceverträge und Laufzeitbild-Tags, gekoppelt mit CLI-Versionen.

Interne Go-Pakete sind keine öffentliche API, es sei denn, explizit dokumentiert.

Das Manifest v1 bleibt die unterstützte v0.4 Kompatibilitätsoberfläche. v0.4 fügt anbieterneutrale Kontrakt-/Laufzeitnähte hinter dieser Oberfläche hinzu, anstatt Anwendungen zu zwingen, ihr Manifest für zukünftige Anbieter umzuschreiben.

## Entwicklung versus Veröffentlichungen

BaseHarbor nutzt zwei langlebige Niederlassungen mit unterschiedlichen Verantwortlichkeiten:

- `main ` ist die freigegebene Quellleitung. Sie sollte die derzeit veröffentlichte Veröffentlichung darstellen und sollte nur durch eine Freigabe PR von`develop` oder ein explizit dokumentiertes Hotfix.
- `develop ` ist der Integrationszweig für das nächste Release. Normales Feature, Fix, Aufgaben und Abhängigkeitszweige Ziel`develop`.

Veröffentlichte Kanäle:

- Tag/Freigabe von GitHub `vX.Y.Z` oder v0.4 Hotfix `vX.Y.Z.H`: unveränderliche unterstützte Veröffentlichung erstellt aus ` main`;
- `ghcr.io/mcpdev80/baseharbor-runtime:X.Y.Z ` or ` X.Y.Z.H`: passendes Laufzeitbild;
- `ghcr.io/mcpdev80/baseharbor-runtime:latest`: neuestes stabiles Release;
- `ghcr.io/mcpdev80/baseharbor-runtime:edge `: bewegliche Entwicklung aus bauen` develop`.

Normale Entwicklungszweige dürfen nicht angestrebt werden `main` direkt. Hotfix-Zweige starten von `main`, werden durch ` main`, und muss dann zusammengeführt/zurückportiert werden ` develop`so behält die nächste Veröffentlichung den Fix.

## Hotfix-Freisetzungen

Während der aktuellen v0.4 Zeile verwendet eine Notfallkorrektur zu einem bereits veröffentlichten Patch eine vierteilige BaseHarbor Hotfix-Version:

```text
MAJOR.MINOR.PATCH.HOTFIX
```

Beispiel:

```text
v0.4.15 -> v0.4.15.1
```

Hotfix-Workflow:

1. Starten Sie den Hotfix-Release-Zweig aus dem exakt freigegebenen `main` Linie, nie von in-progress `develop`.
2. Verknüpfen Sie jede Korrektur mit dem Problem hotfix release parent.
3. Bewahren Sie den Zweig nur auf: keine unabhängigen Features, Refaktors oder Abhängigkeitsupdates.
4. Verwenden Sie eine fokussierte Validierung bei der Implementierung individueller Korrekturen.
5. Update Changelog, Release Notes und alle betroffenen Operator/Referenz-Dokumentation.
6. Führen Sie das komplette Pre-Release-Gate genau einmal an der Freigabegrenze gegen den finalen Hotfix-Kandidat aus.
7. Öffnen Sie die Hotfix-Release-PR für `main` erst, nachdem der Kandidatennachweis grün ist.
8. Veröffentlichen Sie das unveränderliche vierteilige Tag und das passende Laufzeitbild aus dem zusammengeführten `main` Verpflichten.
9. Verifizieren Sie Release-Artefakte/Beweis, dann forward-port die gleichen Fixes zu `develop`.
10. Entfernt temporäre Hotfix/Validierungszweige erst nach Veröffentlichung und Forward-Port sind abgeschlossen.

Wenn eine vorgeschlagene Änderung die Laufzeitidentität, die anhaltende Migration von Staaten, öffentliche Aufträge oder Architektur über das hinaus verändert, was zur Behebung des freigegebenen Defekts erforderlich ist, schieben Sie sie zurück in die normale Roadmap, anstatt das Hotfix zu erweitern.

Rollback-Regel: Verschieben oder schreiben Sie kein veröffentlichtes Tag. Wenn ein Hotfix selbst defekt ist, geben Sie die Freigabeänderung auf der freigegebenen Zeile zurück und veröffentlichen Sie eine neue Hotfix-Version.

Quellregel: Die Release-Ausgabe und Release-Notes müssen den genauen BaseHarbor-Kandidaten SHA, die externe Demo-Revision, die von der Pre-Release-Validierung verwendet wird, und die erfolgreichen Release-Grenznachweise identifizieren.

## Vorbereitung der Freisetzung

Normale Releases werden auf `develop` vorbereitet und erst nach Prüfung des unveränderlichen Kandidaten nach `main` übernommen. Hotfixes beginnen auf `main`. Der [vollständige Release-Audit](https://github.com/mcpdev80/baseharbor/blob/develop/docs/internal/pre-release-documentation-audit.md) umfasst Implementierung, Ökosystem, EN/DE-Dokumentation, Pages-Kandidatenbuild, Changelog und verständliche Release Notes.

1. Implementierung, Verträge, Dokumentation und Release Notes fertigstellen.
2. Exakte Core-, Demo-, Console- und Connector-Commits festlegen, soweit sie zum Release gehören.
3. Betroffene Tests während der Entwicklung gezielt ausführen und Fehler beheben.
4. Den vollständigen Pre-Release erst nach ausdrücklicher Genehmigung am unveränderlichen Kandidaten ausführen. Normale Pflichtjobs müssen erfolgreich sein; fehlgeschlagene, abgebrochene oder fehlende Pflichtarbeit blockiert.
5. Einen Release-PR mit dem geprüften Release-Inhalt nach `main` öffnen. Promotion, Tagging und Veröffentlichung brauchen jeweils den autorisierten Auftrag.
6. Nach der Freigabe einen unveränderlichen Tag auf dem konkreten `main`-Commit erstellen. Der Release-Workflow führt die wiederverwendbaren vollständigen Pre-Release-Prüfungen am exakten Tag aus und veröffentlicht anschließend die Artefakte.
7. Binärdateien, Prüfsummen, Version/Commit, Runtime-Image, Provenance und veröffentlichte Dokumentation tatsächlich prüfen.

CI, Pre-Release und Release verwenden keine Gate-Manifeste, Evidence-Wiederverwendung, erneute Run-/Job-Prüfung oder zusätzliche Receipt-Freigabe. Der einzige CI-Vermerk enthält Merge-SHA und PR-Nummer; siehe [minimale CI-Receipts](../development/minimal-ci-receipts.md). Builds und Images als Eingaben, unveränderliche Consumer-Pins und veröffentlichte Artefakt-Prüfsummen bleiben erhalten.

### Historische Ökosystem-Eingaben

Die veröffentlichten v0.4.24-/v0.4.25-Aufzeichnungen und ihre unveränderlichen Consumer-Pins bleiben historische Daten. Ihre früheren Browser-Receipts, Run-Artefakte und Freigabemanifeste sind keine aktive CI-Voraussetzung. Aktuelle Prüfungen verwenden die tatsächlichen festgelegten Quellstände und führen die erforderlichen Tests direkt aus.

## Freigabe von Artefakten

Jede Veröffentlichung veröffentlicht:

- `baseharbor_linux_amd64.tar.gz`
- `baseharbor_linux_arm64.tar.gz`
- `checksums.txt`
- GitHub Baubeweisbescheinigungen
- passende versionierte GHCR-Laufzeitbilder

Verifizieren Sie binäre Metadaten mit:

```bash
baha version
```

Überprüfen Sie die Prüfsummen mit:

```bash
sha256sum -c checksums.txt --ignore-missing
```

GitHub Provenienz kann zusätzlich mit:

```bash
gh attestation verify baseharbor_linux_amd64.tar.gz -R mcpdev80/baseharbor
```

## Verbraucherberatung

Eine echte Anwendung sollte nie schweigend folgen `main`. während der ` 0.x`serie, ein Antrag validiert gegen ` v0.4.15`sollte sich normalerweise auf die kompatible Nebenleitung beschränken, z.B.`>=0.4.0 <0.5.0`, es sei denn, es bestätigt absichtlich gegen eine neuere kleinere Freigabe.

Anwendungen, die sich durch die v0.4 Linie bewegen, halten Manifest v1 und die bestehende Compose-Entwicklerreise. v0.4.3 fügte den offenen Provider-Integrationsvertrag und die deterministische Lese-only-Repository-Inspektion hinzu. v0.4.4 hinzugefügt: Provider-neutral verwaltete HTTP/HTTPS-Exposition. v0.4.5 hinzugefügt: Provider-neutrale Semantik für sichere Bindung/Workload-Identität. v0.4.6 hinzugefügt: Anbieter-neutrale Semantik `object-storage.s3/v1` mit einem austauschbaren SeaweedFS Compose-Referenzanbieter, Bucket-Scope-Anmeldeinformationen und authentifizierter S3-Präsenz. v0.4.7 hinzugefügter Provider-neutral `telemetry.otlp/v1`, ein faul geteilter OpenTelemy Collector Referenzanbieter, externe OTLP-Bindung und echte OTLP-Export-Überprüfung. v0.4,8 hinzugefügt ` metrics/v1`, Prometheus-Platzierung/Isolierung, Runtime Resource API-Ausführung und gerichtete Cross-Application-Konnektivität. v0.4.9 hinzugefügte Anbieter-neutrale zentrale Workload-Protokolle durch Loki/Alloy, gerenderte Workload-Sicherheit vor dem Flug und ausführbare Provider-Konformität/Fehlerinjektion. v0.4.10 hinzugefügte generische Anbieter-Beobachtungsfähigkeit und Trace-Speicher, v0.4.11 gehärtete Repository-Adoption und Lifecycle UX, v0.4.12 hinzugefügte Agent-native Maschine/MCP-Oberfläche, v0.4.13 hinzugefügte explizite Umgebungs- und Richtliniensemantik, v0.4.14 abgeschlossene typisierte Reconciliation/Eigentum/Konvergenzse-Semantik, v0.4.15 hinzugefügte erstklassige Targets, native Podman-Quadlet-Ausführung, die TLS/Service-Zugangsbasis, komplette Anbieter/Laufobservabilität Semantik, der vollständige Agent-native MCP/JSON-Lebenszyklus und installationsweite vollständige Zerstörung, v0.4.16 hinzugefügte, v0.4.4.16 ausgewählte Anwendungseinheitene Audit/Evidierte Semantik v1 Semantik, v1 hinzugefügte, v.
