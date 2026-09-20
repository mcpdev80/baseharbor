# Releases und Versionierung

BaseHarbor verwendet Semantic Versioning mit Git-Tags im Format `vX.Y.Z`.

## `0.x`-Stabilitaet

Waehrend der `0.x`-Serie gelten Patch-Releases als rueckwaertskompatible Fehler-/Security-Fixes. Minor-Releases duerfen bewusst dokumentierte Breaking Changes enthalten. Anwendungen sollen deshalb nicht `main` verfolgen, sondern einen kompatiblen Versionsbereich pinnen, zum Beispiel fuer die v0.4-Linie:

```text
>=0.4.0 <0.5.0
```

## Release-Kanaele

- `vX.Y.Z`: unveraenderlicher GitHub Release
- `ghcr.io/mcpdev80/baseharbor-runtime:X.Y.Z`: passendes Runtime-Image
- `:latest`: neuester stabiler Release
- `:edge`: beweglicher Entwicklungsstand von `main`

Ein veroeffentlichtes `baha X.Y.Z` verwendet standardmaessig das gleich versionierte Runtime-Image. Ein Development-Build verwendet `edge`. `BASEHARBOR_RUNTIME_IMAGE` bleibt ein expliziter Operator-Override.

## Release-Artefakte

Jeder Release enthaelt Linux-Binaries fuer amd64 und arm64, SHA-256-Checksummen, GitHub-Provenance-Attestations und das passende versionierte Runtime-Image.

```bash
baha version
```

zeigt Version, Commit und Build-Zeit.

## Release-Prozess

Vor jedem Tag gibt es einen Release-Preparation-PR.

1. Der finale Stand wird gegen `docs/DEVELOPMENT_GUIDELINES.md` geprueft, inklusive Ownership, Isolation, Secret-Sicherheit, Fail-closed-Verhalten, Tests und Doku-Konsistenz.
2. Alle betroffenen kanonischen Dokumente werden aktualisiert, inklusive EN/DE-Varianten. Veraltete Versionsnummern, Statusaussagen, Beispiele und Future-Work-Hinweise werden gezielt gesucht.
3. Alle relevanten CI-/Real-Product-Acceptance-Gates muessen auf dem **exakten Release-Preparation-Head** gruen sein. Wo sinnvoll zuerst lokal/Hugging Face pruefen und GitHub Actions nur verwenden, wenn sie fuer den finalen Nachweis erforderlich sind.
4. Der Changelog erhaelt einen datierten Versionsabschnitt.
5. Unter `docs/releases/vX.Y.Z.md` werden menschenfreundliche Release Notes erstellt: Was hat sich geaendert, warum ist es wichtig, welche Kompatibilitaets-/Upgrade-Auswirkungen und Security-Aspekte gibt es und was bleibt bewusst spaeter. Eine rohe Commit-Liste oder ein generiertes Git-Log ist kein Release-Text.
6. Der PR wird erst danach nach `main` gemerged.
7. Der Tag muss exakt auf diesem gruenen `main`-Commit liegen und wird unveraenderlich gepusht.
8. Der Release-Workflow muss erfolgreich Tag/Source validieren, den getaggten Code testen und GitHub Release, Artefakte und Provenance veroeffentlichen.
9. Erst nach Verifikation von GitHub Release, Binaries, Checksums, Provenance und passendem Runtime-Image gilt der Release als abgeschlossen. Ein gepushter Tag allein reicht nicht.

Veroeffentlichte Tags werden niemals verschoben. Fehlerhafte Releases werden durch einen neuen Patch-Release korrigiert.

## v0.4.9-Kompatibilitaet

Manifest v1 bleibt unveraendert. v0.4.9 fuegt zentrale Workload-Logs als Deployment-/Plattform-Policy hinzu, nicht als Loki-Feld im Application Contract. Loki/Alloy, Provider Placement und Collector-State bleiben Provider-/Operator-State.

Der Release erweitert ausserdem den gerenderten Compose-Security-Preflight und macht Provider Integration Contract v1 Lifecycle-Conformance ausfuehrbar. Kubernetes/OpenShift werden dadurch nicht implementiert.
