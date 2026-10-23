# Q14 — Release-Note Grundlegung v0.4.26 (evidence, read-only)

**Coordinator:** Flash · **Worker:** Ornith (qwen3.6) · **Datum:** 2026-10-20
**Auftrag:** Read-only release-note groundwork for v0.4.26. Nur dieses Evidence-File
schreiben; kein Produktcode, keine CHANGELOG-/Release-Note-Änderung.

## 1. Anchor und Bereich

- Anchor = git-Tag `v0.4.26` (existiert, bereits auf `origin/develop`), Commit
  `54acc4b6` (annotiertes Tag), **nicht** `v0.4.25`.
- Befehl (aus `baseharbor/`): `git log --oneline v0.4.26..origin/develop` → leer.
  Der Anchor **ist** die v0.4.26-Spitze auf develop; die seit v0.4.25 gemergten
  Änderungen liegen in `v0.4.25..v0.4.26` (200 Commits bis `origin/develop`
  insgesamt; v0.4.26 ist der letzte Merge-/Release-Punkt auf develop).
- Nur MERGED (PR-)Änderungen seit v0.4.25 wurden gruppiert. Reine Release-/CI-/
  Docs-/Evidence-/GC-/Review-Kommits (`[v0.4.26]`, `ci(...)`, `docs:`,
  `Reviews:`, `Runners:`, `GC:`, `Evidence:`) wurden als nicht user-facing
  herausgefiltert.

## 2. Gruppierte MERGED-Features (seit v0.4.25)

Alle unter „Added" (neue Benutzeroberfläche seit v0.4.25; kein Fix-Schwerpunkt,
keine Inkompatibilitäten).

### Group A — Needs-first App-Setup-/New-Wizard (core-910, core-912, core-913, core-916, core-917, #972/#963/#966/#972/#975)
- Commits: `e514a8b6` (core-916 read-only wizard controller), `30d635b8`
  (core-912 needs-first selection), `b3428843` (core-913 Target-scoped provider
  choices), `4bc21c4d` (New wizard + persistente Target-Präferenzen).
- Signatur: Entwickler-Verträge minimal (`version: 1`, `app.name`,
  `app.environment`), keine应用-UUID; jeder Bedarf „Automatic (recommended)"
  oder explizite Wahl kompatibler Provider/Instanzen desselben Target.
- Write-Set: `cmd/baha/app_*_wizard*.go`, `app_wizard_controller.go`.

### Group B — `baha app edit`: Intent-Diff, Preview, Impact-Consent (core-911, core-917, core-918, core-920, #967/#978/#970/#979)
- Commits: `1c78c509` (core-911 hydration existing selections), `f2bad5ef`
  (core-918 confirmed edits + Apply-Lifecycle), `beca492d` (core-919 workload
  identity retention), `18e4f1d9` (core-920 in-process machine adapters).
- Signatur: `baha app edit [NAME]` zeigt exaktes Intent-Diff + Auswirkungen,
  manuelle Canon/Shorthand-Editierung läuft über denselben Reconcile; Edit
  nutzt den einmaligen Apply-Lifecycle, keine Fabrication von Apply-Receipts.
- Write-Set: `cmd/baha/app_edit*.go`.

### Group C — `baha app footprint show`: autoritative, nicht-mutierende Fußspur (core-914, core-919, #962/#965)
- Commits: `30d635b8` (core-914 read-only presenter), `a1e58f1f` (core-919
  human + JSON).
- Signatur: `baha app footprint show [NAME] [-o json] [--details]` zeigt
  footprint via resolver v0.4.25; berechnet/quantifiziert nicht (kein RAM-/
  Container-Summen-Default), CLI + Plan-JSON + MCP/HTTP teilen denselben
  `baseharbor.footprint/v1`-Resolver mit typed confirmation/denial.

## 3. Sektionen im CHANGELOG-Schema (v0.4.26, da alle User-facing „Added")

- **Added** — Group A (needs-first wizard), Group B (`app edit`), Group C
  (`app footprint show`).
- **Changed / Fixed / Compatibility** — seit v0.4.25 keine isolierten,
  user-facing PR-Merges mit diesem Schwerpunkt identifizierbar; nur
  CI/Release/Cleanup. Keine Zeilen.
- Kein `Security`-/`Support boundaries`-Block erforderlich (kein sicherheits-
  relevanter Produkt-PR-Merge seit v0.4.25).

## 4. EN/DE-Handling (wie Bestand)

- Produkt-PRs bringen EN- und DE-Doku parallel; DE ist Pflicht-Parallelübersetzung.
- Belege (in v0.4.26-Baum, `HEAD`):
  - `docs/cli/command-index.md` und `docs/de/cli/command-index.md` listen
    `baha app edit` (Z.23 EN/DE) und `baha app footprint` / `baha app footprint show`
    (Z.27–28 EN/DE).
  - `docs/cli/applications.md` (Z.127) und `docs/de/cli/applications.md` (Z.97–98)
    dokumentieren `footprint` (`baseharbor.footprint/v1`) parallel.
  - Edit-Counterpart (`baha app edit`) in EN/DE command-index bestätigt;
    Applications-Doku EN/DE für Edit-Zusammenhang in Vorbereitung (core-922).
- Release-Notes selbst: EN-Text primär; DE-Übersetzung als Parallelprodukt
  analog v0.4.25 (kein Ersetzen, keine maschinelle Ersatzsprache).

## 5. PENDING-Platzhalter (Nicht merged, nur Platzhalter)

- **core-921** — Integration des Command Trees in Machine/MCP-Registry,
  Help, Discovery/Coverage-Goldens, generierte Referenzen. PENDING.
- **core-922** — öffentliche EN/DE-Anleitung zur v0.4.26-Oberfläche (Group A/B/C).
  PENDING (Doku-Parallelsetzung, Group A/B/C).
- **core-923** — zielgerichtetes Produkt-/Consumer-Akzeptanz-Verifikation der
  integrierten Oberfläche. PENDING.
- **#998** (core-921-Integration) — offen, NICHT merged. PENDING.
- Hinweis: v0.4.25-Foundation (Core-less lifecycle, Manifest v1,
  capability/provider resolution) ist v0.4.25-Akzeptanz, **nicht** v0.4.26-Work
  → nicht unter v0.4.26 „Added" aufführen.

## 6. Quellen / Belege

- `baseharbor/CHANGELOG.md` — Schema + vorgänger Release-Note-Struktur (0.4.25).
- `baseharbor-plan/releases/v0.4.26.md` — Release-Inventar, Aufgaben-/Abhängigkeits-
  Graph, PENDING-Zuordnung (core-921/922/923, #998).
- `baseharbor/` git: Tag `v0.4.26`, Commits #972/#963/#966/#975/#967/#978/#970/
  #962/#965 (Groupen A/B/C), Doku-Files command-index/applications EN/DE.

Q14-RELNOTE-DONE
