# Releases und Versionierung

BaseHarbor verwendet Semantic Versioning mit Git-Tags im Format `vX.Y.Z`.

## `0.x`-Stabilität

Während der `0.x`-Serie gelten Patch-Releases als rückwärtskompatible Fehler-/Security-Fixes. Minor-Releases dürfen bewusst dokumentierte Breaking Changes enthalten. Anwendungen sollen deshalb nicht `main` verfolgen, sondern einen kompatiblen Versionsbereich pinnen, zum Beispiel:

```text
>=0.1.0 <0.2.0
```

## Release-Kanäle

- `vX.Y.Z`: unveränderlicher GitHub Release
- `ghcr.io/mcpdev80/baseharbor-runtime:X.Y.Z`: passendes Runtime-Image
- `:latest`: neuester stabiler Release
- `:edge`: beweglicher Entwicklungsstand von `main`

Ein veröffentlichtes `baha X.Y.Z` verwendet standardmäßig das gleich versionierte Runtime-Image. Ein Development-Build verwendet `edge`. `BASEHARBOR_RUNTIME_IMAGE` bleibt ein expliziter Operator-Override.

## Release-Artefakte

Jeder Release enthält Linux-Binaries für amd64 und arm64, SHA-256-Checksummen, GitHub-Provenance-Attestations und das passende versionierte Runtime-Image.

```bash
baha version
```

zeigt Version, Commit und Build-Zeit.

## Release-Prozess

Vor jedem Tag gibt es einen Release-Preparation-PR. Alle relevanten CI-/Acceptance-Gates müssen grün sein, der Changelog erhält einen datierten Versionsabschnitt, und der Tag muss exakt auf einem Commit liegen, der Bestandteil von `main` ist.

Veröffentlichte Tags werden niemals verschoben. Fehlerhafte Releases werden durch einen neuen Patch-Release korrigiert.