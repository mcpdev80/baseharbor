# Releases und Versionierung

BaseHarbor verwendet Semantic Versioning mit Git-Tags im Format `vX.Y.Z`.

## `0.x`-Stabilitaet

Waehrend der `0.x`-Serie gelten Patch-Releases als rueckwaertskompatible Fehler-/Security-Fixes. Minor-Releases duerfen bewusst dokumentierte Breaking Changes enthalten. Anwendungen sollen deshalb nicht `main` verfolgen, sondern einen kompatiblen Versionsbereich pinnen, zum Beispiel fuer die v0.4-Linie:

```text
>=0.4.0 <0.5.0
```

## Entwicklungs- und Release-Branches

BaseHarbor verwendet zwei dauerhafte Branches mit klar getrennten Aufgaben:

- `main` ist der veroeffentlichte Source-Stand. Er soll dem aktuell publizierten Release entsprechen und wird nur durch einen Release-PR aus `develop` oder einen expliziten Hotfix fortgeschrieben.
- `develop` ist der Integrations-Branch fuer den naechsten Release. Normale Feature-, Fix-, Chore- und Dependency-Branches zielen auf `develop`.

Normale Entwicklungs-PRs duerfen nicht direkt auf `main` zielen. Hotfixes starten von `main`, werden ueber `main` released und danach in `develop` uebernommen, damit der Fix im naechsten Release erhalten bleibt.

## Release-Kanaele

- `vX.Y.Z`: unveraenderlicher GitHub Release von `main`
- `ghcr.io/mcpdev80/baseharbor-runtime:X.Y.Z`: passendes Runtime-Image
- `:latest`: neuester stabiler Release
- `:edge`: beweglicher Entwicklungsstand von `develop`

Ein veroeffentlichtes `baha X.Y.Z` verwendet standardmaessig das gleich versionierte Runtime-Image. Ein Development-Build verwendet `edge`. `BASEHARBOR_RUNTIME_IMAGE` bleibt ein expliziter Operator-Override.

## Release-Artefakte

Jeder Release enthaelt Linux-Binaries fuer amd64 und arm64, SHA-256-Checksummen, GitHub-Provenance-Attestations und das passende versionierte Runtime-Image.

```bash
baha version
```

zeigt Version, Commit und Build-Zeit.

## Release-Prozess

Jeder Release wird auf `develop` vorbereitet und erst nach erfolgreicher Release-Pruefung nach `main` promoted.

1. Der finale Stand auf `develop` wird gegen `docs/DEVELOPMENT_GUIDELINES.md` geprueft, inklusive Ownership, Isolation, Secret-Sicherheit, Fail-closed-Verhalten, Tests und Doku-Konsistenz.
2. Alle betroffenen kanonischen Dokumente werden aktualisiert, inklusive EN/DE-Varianten. Veraltete Versionsnummern, Statusaussagen, Beispiele und Future-Work-Hinweise werden gezielt gesucht.
3. Der Changelog erhaelt einen datierten Versionsabschnitt.
4. Unter `docs/releases/vX.Y.Z.md` werden menschenfreundliche Release Notes erstellt: Was hat sich geaendert, warum ist es wichtig, welche Kompatibilitaets-/Upgrade-Auswirkungen und Security-Aspekte gibt es und was bleibt bewusst spaeter. Eine rohe Commit-Liste oder ein generiertes Git-Log ist kein Release-Text.
5. Wo sinnvoll wird zuerst lokal/Hugging Face validiert.
6. Der verpflichtende GitHub-Pre-Release-Workflow wird gegen den exakten Release-Candidate-SHA auf `develop` ausgefuehrt. Fehler werden auf `develop` korrigiert und der Gate-Lauf wiederholt, bis alles gruen ist.
7. Danach wird genau ein Release-PR `develop -> main` erstellt. Dieser PR darf keine sachfremden Aenderungen enthalten.
8. Erst nach gruenem Pre-Release-Gate wird nach `main` gemerged.
9. Der Tag `vX.Y.Z` wird auf dem daraus resultierenden `main`-Release-Commit erstellt und unveraenderlich gepusht.
10. Der Release-Workflow muss erfolgreich pruefen, dass der Tag in `main` enthalten ist, den getaggten Code erneut testen und GitHub Release, Artefakte und Provenance veroeffentlichen.
11. Erst nach Verifikation von GitHub Release, Binaries, Checksums, Provenance und passendem Runtime-Image gilt der Release als abgeschlossen. Ein gepushter Tag allein reicht nicht.

Veroeffentlichte Tags werden niemals verschoben. Fehlerhafte Releases werden durch einen neuen Patch-Release korrigiert.

## v0.4.9-Kompatibilitaet

Manifest v1 bleibt unveraendert. v0.4.9 fuegt zentrale Workload-Logs als Deployment-/Plattform-Policy hinzu, nicht als Loki-Feld im Application Contract. Loki/Alloy, Provider Placement und Collector-State bleiben Provider-/Operator-State.

Der Release erweitert ausserdem den gerenderten Compose-Security-Preflight und macht Provider Integration Contract v1 Lifecycle-Conformance ausfuehrbar. Kubernetes/OpenShift werden dadurch nicht implementiert.
