# Veröffentlichungen und Versionierung

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

Evidence-Regel: Die Release-Ausgabe und Release-Notes müssen den genauen BaseHarbor-Kandidaten SHA, die externe Demo-Revision, die von der Pre-Release-Validierung verwendet wird, und die erfolgreichen Release-Grenznachweise identifizieren.

## Vorbereitung der Freisetzung

Normale Freisetzungen werden am `develop` und gefördert zu `main` nur nachdem der Release Candidate nachgewiesen wurde. Hotfix-Releases folgen dem Haupt-basierten Workflow oben.

Die obligatorische End-to-End-Checkliste ist`docs/internal/pre-release-documentation-audit.md` (interne, nicht publizierte Prüfanweisung). Trotz seines historischen Dateinamens handelt es sich um das kanonischen **vollständige Vorab-Release-Audit** und umfasst Umfang/Ausgaben, BaseHarbor-Implementierung, Verträge, EN/DE-Dokumente, Roadmap/Staleness, Changelog/Release-Notes,` baseharbor-demo`, GitHub Pages, Exact-Candidate Evidence, Promotion, Publishing und Post-Release-Verifikation. Ein Release darf Checklisten-Abschnitte nicht überspringen, da der Feature Code oder normale CI bereits grün ist.

1. Überprüfung der endgültigen Umsetzung `develop` dagegen `docs/DEVELOPMENT_GUIDELINES.md`, einschließlich Eigentum, Isolation, Geheimsicherheit, ausfallgeschlossenes Verhalten, Tests und Dokumentationskonsistenz.
2. Überprüfen und aktualisieren Sie alle betroffenen kanonischen Dokumentationen, einschließlich der beiden EN/DE-Varianten, wo sie existieren. Suchen Sie explizit nach veralteten Versionsnummern, Implementierungsstatus-Ansprüchen, Beispielen und Zukunftsaussagen.
3. Verschieben Sie relevante Einträge aus `[Unreleased]` in eine datierte `## [X.Y.Z] - YYYY-MM-DD` Abschnitt in `CHANGELOG.md` (or ` X.Y.Z.H`für einen v0.4 Hotfix).
4. Schreiben Sie menschenlesbare Release Notes unter `docs/releases/vX.Y.Z.md` (or ` vX.Y.Z.H.md`für einen v0.4 Hotfix). Sie sind die kanonischen GitHub Release-Nachricht und müssen erklären, was sich geändert hat, warum es wichtig ist, Kompatibilität/Upgrade-Impact, Sicherheitsimplikationen und absichtlich verschobenes Arbeiten; eine rohe Commit-Liste oder generiertes Git-Log ist keine akzeptable Release-Nachricht.
5. Beenden und verschmelzen Sie die exakte externe `baseharbor-demo` Revision für die Freigabe-Validierung vorgesehen, dann aufnehmen seine unveränderliche 40-Zeichen Commit SHA in `docs/releases/vX.Y.Z.demo-ref` (or ` vX.Y.Z.H.demo-ref `). Umzug` baseharbor-demo/main`ist nie ein akzeptabler Auslösestift.
6. Überprüfen Sie den Kompatibilitätseffekt und wählen Sie den dokumentierten Release-Versionsschritt aus.
7. Starten Sie zunächst die lokale/hugging Face-Validierung, wenn dies praktisch ist.
8. Erst nach der Implementierung, Dokumentation, CHANGELOG, README, menschenlesbare Release Notes, die externe `baseharbor-demo` und die versionierte Demo-Ref-Datei sind komplett, führen Sie den obligatorischen GitHub Pre-Release-Workflow gegen die genaue Release-Candidate SHA. Pre-Release ist ein Orchestrator über unabhängig rerunnable Atomic Gates: Static Contract/DX Gates laufen ohne Containerlaufzeit, während Docker und Podman Gates erklärt minimale Ressourcenprofile. Jedes Gate lädt Beweise gebunden an den genauen BaseHarbor SHA, Demo SHA, Laufzeit und Gate Namen.
9. Wenn die Basislinie Fehler findet, lassen Sie erfolgreiche Gates nicht erneut laufen. Befestigen Sie einen Ausfallbereich zu einem Zeitpunkt und starten Sie genau das gescheiterte Atomgate durch `Targeted Atomic Gate` mit dem unveränderten Kandidaten/Demo SHA, bis er grün ist. Eine Codeänderung entwertet die betroffenen Beweise und verlangt, dass die entsprechenden Gates erneut ausgeführt werden; nicht zusammenhängende grüne Beweise werden nicht nur deshalb verworfen, weil ein anderes Gate fehlgeschlagen ist.
10. Sobald alle Atomtore einzeln grün auf dem letzten unveränderten Kandidaten sind, führen Sie den kompletten Vorab-Orchester einmal als Freigabe-Grenzgenehmigung. Das endgültige Beweismanifest muss das komplette zu erwartende Atomtor für den genauen BaseHarbor Kandidaten SHA enthalten und externe Demo SHA pinned.
11. Öffnen Sie eine Freigabe PR von `develop` to ` main`. Mischen Sie keine unabhängigen Änderungen in dieser PR.
12. Zusammenführen `develop -> main` erst nachdem das Pre-Release-Gate grün ist und der Release diff verstanden wird.
13. Erstellen Sie das unveränderliche Release-Tag auf dem resultierenden `main` Commit freigeben und schieben.
14. Der Release-Workflow verbraucht die erfolgreiche, unveränderliche Pre-Release-Zulassung, anstatt die gleiche Source/Runtime/Demo-Akzeptanz-Suite zu wiederholen. Er führt nur Release-Only-Checks durch, die noch nicht abgedeckt sind, veröffentlicht das passende Runtime-Image, GitHub Release, Archive und Provenienz.
15. Überprüfen Sie die resultierende GitHub Release, Binärdateien, Prüfsummen, Provenienz, passendes Laufzeitbild und referenzierte Pre-Release-Evidenz, bevor Sie die Veröffentlichung für nutzbar erklären.

Verschieben Sie niemals ein veröffentlichtes Versions-Tag. Beheben Sie eine schlechte Version mit einem neuen Patch-Release.

### Unveränderliche Ökosystem-Eingaben für v0.4.24

Für v0.4.24 bindet die `integration-candidate.json` des finalen Connector-Commits die exakten Core-, Console- und Demo-Commits. Das Console-Manifest muss denselben Core und dieselbe Demo binden; die `baseharbor-core.ref` der Demo muss diesem Core-Commit entsprechen. Dies ersetzt für v0.4.24 die Core-eigene `.demo-ref`: Das gegenseitige Pinnen zukünftiger Git-Commits würde eine zyklische Abhängigkeit erzeugen.

`scripts/ecosystem_release_pins.py` prüft alle Bindungen und erstellt das öffentliche Manifest der Consumer-Nachweise. Der Pre-Release-Workflow akzeptiert eine optionale exakte `consumer_ref` oder löst den geprüften Connector-Integrationsbranch einmalig auf und hält dessen 40-stelligen SHA fest. Die Freigabe bewahrt diese unveränderliche Consumer-Quelle; die Release-Nachprüfung löst dieselben Commits auf. Der vollständige native Connector-Job prüft auch das exakte Console-Browser-Receipt. Sein authentifiziertes Artefakt kann deshalb die Console-Integration ohne Wiederholung derselben Journey belegen. Alle bisherigen atomaren Gates bleiben erforderlich.

Das Vorbereiten dieser Eingaben startet oder genehmigt keinen Pre-Release. Die finale SHA-Matrix und die gezielten Entwicklungsabnahmen stehen in [PR #837](https://github.com/mcpdev80/baseharbor/pull/837). v0.4.24 bleibt bis zur gesonderten Freigabe Unreleased.

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
