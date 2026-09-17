# Releases und Versionierung

BaseHarbor verwendet Semantic Versioning mit Git-Tags im Format `vX.Y.Z`.

## `0.x`-Stabilitaet

Waehrend der `0.x`-Serie gelten Patch-Releases als rueckwaertskompatible Fehler-/Security-Fixes. Minor-Releases duerfen bewusst dokumentierte Breaking Changes enthalten. Anwendungen sollen deshalb nicht `main` verfolgen, sondern einen kompatiblen Versionsbereich pinnen, zum Beispiel fuer die v0.3-Linie:

```text
>=0.3.0 <0.4.0
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

1. Alle relevanten CI-/Real-Product-Acceptance-Gates muessen auf dem exakten Release-Preparation-Head gruen sein.
2. Der Changelog erhaelt einen datierten Versionsabschnitt.
3. Der PR wird erst danach nach `main` gemerged.
4. Der Tag muss exakt auf diesem gruenen `main`-Commit liegen.
5. Nach der Veroeffentlichung werden Binaries, Checksums, Provenance und die CLI-/Runtime-Image-Versionskopplung verifiziert.

Veroeffentlichte Tags werden niemals verschoben. Fehlerhafte Releases werden durch einen neuen Patch-Release korrigiert.
