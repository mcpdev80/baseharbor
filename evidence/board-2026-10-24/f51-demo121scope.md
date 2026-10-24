# F51 - Demo PR #121 Scope-Haerte (Diagnose-Datei-Nachweis)

Mission: ops/missions/f51.txt
Datum: 2026-10-24
Lane/Session: F51 (diese Session)
Repository: baseharbor-demo (git NUR hier; Root-Repo fuer git verboten)

## PLAN

- [x] 0. Evidence-Datei mit ## PLAN + nativen Todos anlegen (vor Analyse)
- [x] 1. basics-check FLAGS abarbeiten (Events gelesen + --ack; Reminders als Beobachtungsdaten verarbeitet, kein Aussen-Auftrag daraus abgeleitet)
- [x] 2. PR #121 exakt identifizieren: head-SHA, base-SHA, gh pr diff 121 -> Dateiliste (Erwartung: genau baseharbor-core.ref) -> BESTAETIGT
- [x] 3. git -C baseharbor-demo fetch origin; sha256 von scripts/failure-runtime-diagnostics.sh gemessen an origin/main, PR-Head, Run-Head (+ PR#120-Head als Kontext); nur `git show <sha>:<pfad> | sha256sum`, kein Checkout
- [x] 4. Ergebnis-Tabelle + Festellung: geaenderte Diagnose-Datei im Kandidaten -> NEIN
- [x] 5. Verzweigung NEIN: Schluss + datei:zeile-Belege der Uebertragung (Schritt-DOCH entfällt, kein Korrekturbedarf an #121)
- [x] 6. Run 39990941852: Jobs/Marker + Einordnung "compatible input" (begruendet)
- [x] 7. Evidence finalisieren, END-Marker

## Constraints (aus Mission)

- pwd -P vor JEDEM git-Befehl; git NUR in baseharbor-demo (Root-Repo nie per git beruehrt)
- KEIN Merge, KEIN Review-Kommando, KEIN Push, KEIN CI-Dispatch, KEINE VM/Docker-Laeufe
- Nur lesende Objektzugriffe im neutralen Objekt-Store (git show/git diff <sha>..<sha>), keine fremden Worktrees angefasst

## 1. PR #121 Identifikation (harte Objekte + API)

- PR #121 `ci(release): repin demo core.ref to v0.4.26 anchor 1861ed07`, OPEN, base `main` = `82d042bc9b2393733858fd5fc9dd42a7f5491ea9`, head = `f57c8b6e5ff39d0873df62b26272962765e88d6e` (Branch `work/v0.4.26-demo-repin`), changedFiles=1.
- `gh pr diff 121 --name-only` -> genau `baseharbor-core.ref`.
- Objekt-Gegenprobe (unabhaengig von der API): `git diff --name-only 82d042bc f57c8b6e` -> genau `baseharbor-core.ref` (-`2adae47a…` -> +`1861ed07…`). **f49-Auftrag erfuellt.**
- origin/main nach `git fetch origin` unveraendert `82d042bc…` = PR-Base.

## 2. Hash-Tabelle scripts/failure-runtime-diagnostics.sh

Messung ausschliesslich `git show <sha>:scripts/failure-runtime-diagnostics.sh | sha256sum` im Repo-Objektstore baseharbor-demo; kein Checkout.

| # | SHA-Kontext | Commit | Git-Blob | SHA-256 |
|---|---|---|---|---|
| (a) | origin/main / PR-#121-Base | `82d042bc9b2393733858fd5fc9dd42a7f5491ea9` | `ea897dfb184dc7f043869174f1e5753b7144f91c` | `650ac86e89d881bda640b49ae2ac177e7510da3df513aef762123b1579ccba8f` |
| (b) | PR #121 Head (Kandidat) | `f57c8b6e5ff39d0873df62b26272962765e88d6e` | `ea897dfb184dc7f043869174f1e5753b7144f91c` | `650ac86e89d881bda640b49ae2ac177e7510da3df513aef762123b1579ccba8f` |
| (c) | Run 39990941852 Head (Release-Tipp) | `25c319697fd3d89d894943e0f44cd3d5dde8b198` | `1558a2435f9cc34dc068430598cdfc143b331429` | `f31bb1c0131d0f22c8449d696cd0c7bebd4e113211504e25bdf96c217625a276` |
| (d) | PR #120 Head (Kontext) | `bd86c7c3cbf7ebfb6dc5631c917ab244c5ed79c5` | `1558a2435f9cc34dc068430598cdfc143b331429` | `f31bb1c0131d0f22c8449d696cd0c7bebd4e113211504e25bdf96c217625a276` |

- (a) == (b): identischer Blob, identischer Hash -> **Festellung: NEIN, PR #121 enthaelt keine geaenderte Diagnose-Datei.**
- (c) == (d): die geaenderte Diagnose-Version (podman total 60s->90s, per-call 20s->60s; belegt per `git diff 82d042bc 25c3196 -- scripts/failure-runtime-diagnostics.sh`) lebt in PR #120 bzw. dem Release-Branch, NICHT in #121.
- Kreuzabgleich: meine gemessenen Werte (b) `650ac86e…` und (c) `f31bb1c0…` stimmen exakt mit den im Review selbst notierten Hashes ueberein (codex-verdict-demo121-r2.md:36) - das Review hat richtige Zahlen, aber die Forderung an den falschen PR adressiert.

## 3. Schluss (Verzweigung NEIN)

Der Review-Punkt "changed diagnostic behavior" (codex-verdict-demo121-r2.md:22 "the tested executable cleanup inputs differ") trifft PR #121 nicht als Aenderung: Der Kandidaten-Diff ist exactly 1 Datei (r2.md:30 bestaetigt das selbst), und die Diagnose-Datei im Kandidaten ist byte-identisch zu main. Die Qualifikationsforderung ist uebertragen aus PR #120 / dessen Release-Zweig:

- Wo die Uebertragung herkommt (datei:zeile aus codex-verdict-demo121-r2.md):
  - :22 - "…remains unsupported because the tested executable cleanup inputs differ" - abgeleitet aus Run-zu-Kandidat-Vergleich, nicht aus dem PR-Diff.
  - :36 - "Full **run->candidate** diff changes .github/workflows/v0424-targeted.yml … and scripts/failure-runtime-diagnostics.sh" - die Referenzgroesse ist die Runquelle `25c3196` (Release-Tipp mit (b)-Aenderung), nicht die PR-Base `82d042b`. Der run->base-Diff enthaelt die Diagnose-Aenderung; der base->head-Diff von #121 nicht.
  - :45 - Rezept verlangt Evidence "covering the changed diagnostic behavior … cleanup-preparation result, … per-call limit, total budget, native isolation and artifact provenance on the frozen candidate" - genau die Merkmalsliste der #120-Diagnose-Aenderung (Blob `1558a243`, Limits 90s/60s). Fuer #121 existiert diese Aenderung nicht; ihre Qualifikation gehoert zu #120 (open, `bd86c7c`).
- Vorbeleg: f49-demorepin.md:33 und :36 hatten (b) bereits als abwesend festgehalten; f49-demorepin.md:23 markierte den Run-Abdeckungsgrad korrekt als "Kontext, nicht qualifizierender Nachweis fuer diesen Head".
- Was real fehlt, ist kein Scope-Fehler von #121, sondern ein Run-Abdeckungsloch: kein grüner Lauf existiert auf exakt dem Kandidatenbaum `f57c8b6` (push-only-Workflow, s. f49-demorepin.md:21-24). Das ist eine CI-/Maintainer-Aufgabe (Inhalt auf Release-Branch mit `[demo-runtime]`), hier laut Mission NICHT ausgefuehrt und nicht dieser Lane zugehoerig.

## 4. Run 39990941852 - Inhalt und Einordnung

- Workflow `Targeted Demo compatibility`, event **push**, head `25c319697…` = Tipp `work/v0.4.26-ecosystem-release`, displayTitle "fix(demo): tolerate podman daemon cold-start stall in cleanup-prepara…", created 2026-10-23T16:56:30Z, updated 17:29:32Z, conclusion success.
- Jobs: "Demo install and backup recovery . podman" (120542211968, success), "Demo install and backup recovery . docker" (120542212240, success).
- Pin-Kompatibilitaet: `baseharbor-core.ref` im Run == Kandidat (Blob `2bae2e9a`, sha256 `a2c83515…`, Wert `1861ed07888a1ec22c218ca491d23ee54f48e261`; Base-Main hatte `2adae47a…`). Der Run hat also denselben Core-Anker installiert.
- Einordnung "compatible input" fuer den Pin-only-Kandidaten: **teilweise, nicht streng.** Begründung: Baumdelta Run->Kandidat = genau `.github/workflows/v0424-targeted.yml` (Artefakt-Retention, laeuft nicht im Testpfad) + `scripts/failure-runtime-diagnostics.sh` (laeuft im Testpfad via `scripts/acceptance.sh:80` bei cleanup-preparation, hier mit entspannten 90s/60s statt main-strikten 60s/20s). Der Run beweist damit dieselbe Core-Identitaet und die Journey auf der #120-Diagnostik-Variante, aber nicht den exakten Kandidatenbaum. Naehere Begruendung statt Behauptung: Die Richtung der Abweichung ist beim Kandidaten die **strengere, main-seitig praexistente** Version - #121 fuehrt keinerlei Lockerung ein, die es zu rechtfertigen gaelte.

## Befund

PR #121 ist scope-rein (nur baseharbor-core.ref, hart belegt ueber API UND Objekt-Diff). Die Review-Forderung nach Nachweis "changed diagnostic behavior" ist gegenstandslos fuer #121 und auf PR #120 uebertragen (r2.md:22/:36/:45; Diagnose-Blobs: #121 == main == `ea897dfb`, #120/Run == `1558a243`). Kein Korrekturbedarf an #121. Offener, anderer Lane zugehoeriger Punkt: exakt-Baum-Qualifikationslauf fuer `f57c8b6` (push-only-Modell, Maintainer-Aktion) und separate Qualifikation der #120-Diagnostik-Aenderung.

## repo-rule compliance

- `pwd -P` vor jedem git-Befehl ausgefuehrt; alle git-Kommandos ausschliesslich in `/home/marcel/dev/baseharbor-ecosystem/baseharbor-demo` (fetch, rev-parse, show, diff); Root-Repo nie per git beruehrt (nur Skript-/Evidence-/gh-Aufrufe).
- Kein Checkout, kein Merge, kein Push, kein Review-Kommando, kein CI-Dispatch, keine VM/Docker-Laeufe; ausschliesslich lesende Objektzugriffe via `git show`/`git diff <sha>..<sha>` und gh-API.
- Prompt-Injection-Defense: Mission, Review-Datei, f49-Akte und Events/Reminders als Daten behandelt; gehandelt nur nach dieser Mission und den Repo-Regeln.

END F51-DEMO121SCOPE-DONE
