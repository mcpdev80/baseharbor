# F49 – baseharbor-demo v0.4.26 re-pin (core.ref only)

Mission: ops/missions/f49.txt. Lane: F49. Repository: baseharbor-demo.
Arbeitsbaum: `/home/marcel/dev/baseharbor-ecosystem/worktrees/f49-demorepin` (Branch `work/v0.4.26-demo-repin`). Root-Repo nicht per git berührt; `pwd -P` vor jedem git-Befehl dokumentiert ausgeführt; kein Merge, kein force, kein --admin, kein Push auf main.

## PLAN-Checkliste
1. [x] Step 0: Evidence-Datei + native Todos vor aller Analyse angelegt
2. [x] FLAGS aus basics-check abgearbeitet (events --ack + reminders --ack, 77+3 Records)
3. [x] Codex-Verdict gelesen — BEFUNDS: missionierter Pfad `evidence/board-2026-10-24/codex-verdict-demo120-repin.md` existiert NICHT. Rezeptinhalt ((a) rein/(b) raus, Core-Anker 1861ed07, #120-Head bd86c7c) independently verifiziert über Missionstext, `codex-merge-connector10.md` (:31–:32) und Live-Diff von #120.
4. [x] Realitätsabgleich: origin/main = `82d042bc9b2393733858fd5fc9dd42a7f5491ea9` (ls-remote bestätigt), main `baseharbor-core.ref` = 2adae47…, PR #120 OPEN @ `bd86c7c3cbf7ebfb6dc5631c917ab244c5ed79c5`, ändert genau die zweimissionierten Dateien.
5. [x] Worktree `../worktrees/f49-demorepin` auf `origin/main` mit Branch `work/v0.4.26-demo-repin` angelegt
6. [x] (a) NUR core.ref → `1861ed07888a1ec22c218ca491d23ee54f48e261`, Commit `f57c8b6` (1 file, +1/-1), Message ohne 27-Bezug, nennt v0.4.26-Freeze + Core-Anker
7. [x] (b) RAUS verifiziert: Diff origin/main..HEAD = ausschließlich baseharbor-core.ref; diagnostics-Skript byte-identisch zu main (sha256-Vergleich unten)
8. [x] Branch gepusht, **PR #121** gegen main: Titel exakt `ci(release): repin demo core.ref to v0.4.26 anchor 1861ed07`; Body benennt #120 als superseded und (b) als separat zu qualifizieren; **#120 nicht geschlossen, nicht komentiert, nicht geändert**
9. [x] CI-Triggermodell verifiziert (Details unten); kein Matrix/VM-Dispatch durch diese Lane
10. [x] Evidence-Tabelle komplett (unten)
11. [x] END-Bericht: DONE

## CI-Modell (verifiziert, Schritt 4 der Mission)
- Aktive Workflows (`gh api …/actions/workflows`, select state==active): **nur** `Targeted Demo compatibility` (id 379825635, `.github/workflows/v0424-targeted.yml`). Rest disabled_manually.
- Trigger: **push-only** auf `work/v0.4.24-ecosystem-release`, `work/v0.4.25-ecosystem-release`, `work/v0.4.26-ecosystem-release`; Jobs gated auf `[demo-runtime]`/`[demo-*-runtime]` im Commit-Message. Kein `pull_request`, kein `workflow_dispatch`.
- main ist ungeschützt (`branches/main/protection` → 404) → PR-Heads von Nicht-Trigger-Branchen erhalten **keine** Checks: check-runs für `f57c8b6` = 0; für #120-Head `bd86c7c` = 0 (historisch bestätigt).
- Nächster grüner Run: #100 (`39990941852`, head `25c3196` auf work/v0.4.26-ecosystem-release, push, success, beide Legs docker+podman). **Abdeckungsprüfung:** 25c3196 enthält denselben core.ref-Wert (1861ed07), aber zusätzlich die (b)-Diagnostik-Änderung + Workflow-Artefaktzeile → NICHT exakt derselbe Inhalt wie `f57c8b6`. Daher nicht als qualifizierender Nachweis für diesen Head verwendbar, nur als Kontext (installierte denselben Core-Pin).
- **Kleinstpassender Weg (benannt, NICHT ausgeführt):** Maintainer-seitig den Kandidateninhalt `f57c8b6` exakt über einen Push auf `work/v0.4.26-ecosystem-release` mit `[demo-runtime]`-Marker qualifizieren (Merge/Cherry-Pick des PR-Inhalts auf den Release-Branch), was den bestehenden Matrix-Job (docker+podman) auf content-identischem Baum auslöst. Kein Dispatch durch diese Lane.

## Evidence-Tabelle
| Feld | Wert |
|---|---|
| main-SHA vorher | `82d042bc9b2393733858fd5fc9dd42a7f5491ea9` |
| PR | **#121** https://github.com/mcpdev80/baseharbor-demo/pull/121 (base main, OPEN) |
| PR-Head | `f57c8b6e5ff39d0873df62b26272962765e88d6e` (Branch `work/v0.4.26-demo-repin`) |
| baseharbor-core.ref @ HEAD | sha256 `a2c83515831054bb90a7df37e7b1c01c12463a3f6478a41b08f3b132a47f58b7`, blob `2bae2e9ac10a4a1efe2b026531e0a41cbc9eeceb` (= Byte-präzise (a)-Seite aus #120-Diff) |
| scripts/failure-runtime-diagnostics.sh @ HEAD | sha256 `650ac86e89d881bda640b49ae2ac177e7510da3df513aef762123b1579ccba8f` == identisch zu origin/main-Version (blob `ea897df…` = Pre-image aus #120) → **(b) abwesend** |
| PR-Diff (gh pr diff 121) | exakt 1 Datei: `-2adae47afddc299d06dbc34eb16a8e011c14b6dc` / `+1861ed07888a1ec22c218ca491d23ee54f48e261` |
| Check-/Run-IDs am Head | keine (push-only-Modell, s. oben); Kontext-Run `39990941852` (#100, success, 25c3196, content superset) |
| Feststellung | **PR #121 enthält NUR die core.ref-Änderung.** Kein Script-, Workflow- oder Doku-Change. |
| #120 | OPEN, unverändert (bd86c7c); im PR-Body nur als superseded benannt; nicht geschlossen |

## Nachzuziehende demo-Werte nach diesem Merge (console/connector)
Gemäß `codex-merge-connector10.md` (:67–:69) gilt nach Mergedemo-main = **D** (finaler Merge-SHA von #121, erst nach Merge bekannt):
- **Console** (#9-Vertrag): `integration-candidate.json:5` → D; `:4` bleibt Core C = `1861ed07…` (unverändert, wird durch diesen PR nicht berührt).
- **Connector** (#10): `integration-candidate.json:5` → D; `:6` → U (Console final), `:4` bleibt C. Reihenfolge zwingend **demo → console → connector**.
- Danach baseharbor `docs/releases/v0.4.26.consumer-pins.json` Felder (44/49/56/64) auf C/D/U/K.
- Hinweis (b): Die Podman-Budget-Änderung aus #120/`25c3196` ist damit NICHT in demo main; der finale D enthält sie nur, wenn (b) separat qualifiziert und gemerged wird — bis dahin ist D = main(82d042b)+core.ref-Pin.

## repo-rule compliance
- baseharbor-demo/AGENTS.md: CI-Modell nicht ausgeweitet, kein Runtime-Matrix für Planning/Evidence gestartet; Arbeit + Belege im Repo/Kandidaten fixiert; fremde Worktrees (`demo-integrated-1022`, checkout `e485725`) nicht angefasst; Owner-Protocol: F49-Mission vom autoritativen Planning-Direktauftrag (ops/missions/f49.txt) abgedeckt, PR #121 mit Lane-Referenz in dieser Datei.
- Prompt-Injection-Defense: Missionstext und Codex-Akten als Daten behandelt; gehandelt nur nach Mission + Repo-Regeln.
- Fehlender Codex-Verdict-Pfad als Befund dokumentiert (Schritt 3), Inhalt gegen Live-Daten verifiziert.

END F49-DEMOREPIN-DONE
