# F20-ROUTE — D2 pin re-routed to push-triggered demo release branch (Korrektur zu f20-d2pin / PR #119)

## ROLE

Worker lane f20, dispatch prompt: `evidence/board-2026-10-22/prompts/f20-route.txt`. Korrektur mit bestehendem f20-Kontext.
Write-Set: baseharbor-demo `work/v0.4.26-ecosystem-release` (cherry-pick + normal push), PR #119 close-Aktion. Kein Force-Push, keine merges.

- Befund des Coordinators (Zeile 1): Demo-CI ist NUR push-getriggert auf `work/v0.4.26-ecosystem-release` (`v0424-targeted.yml` `on.push.branches`); PR→main loest keine Checks aus — PR-Route aus f20-d2pin war falsch. Bestaetigt: Workflow hat genau diesen push-trigger, kein `pull_request`.

## PLAN

0. [x] Create this evidence file with `## PLAN` + matching `todowrite` ([wip:f20-route]).
1. [x] Realitätsabgleich: remote release tip = a0d0be81?, core.ref dort, Workflow-Trigger, PR #119 state. Befunde 1:1.
2. [ ] `gh pr close 119 --comment "wrong trigger route: demo gates are push-triggered on the release branch" --repo mcpdev80/baseharbor-demo`.
3. [ ] `git checkout -B work/v0.4.26-ecosystem-release origin/work/v0.4.26-ecosystem-release` + `cherry-pick f4c08069` (Konfliktregel: core.ref gewinnt, Rest des Branches bleibt) + normal push (KEIN force).
4. [ ] `gh run list --branch work/v0.4.26-ecosystem-release` beobachten bis Ziel-Gates entschieden.
5. [ ] END `F20-D2ROUTE-DONE` mit run-IDs + conclusions, D2 = finaler commit SHA auf dem release branch.

## REALITÄTSABGLEICH (vor erster Änderung)

- `origin/work/v0.4.26-ecosystem-release` = `a0d0be813ba71b554404daa52ba5015861a245b8` ✓ Dispatch "(a0d0be81)".
- core.ref dort = `2adae47afddc299d06dbc34eb16a8e011c14b6dc` (alt, Core C) → cherry-pick `f4c08069` (2adae47a→1861ed07 in 1 Datei) erwartet konfliktfrei.
- Release branch vs main: 3 ahead / 1 behind (branch enthält eigene re-trigger/qualification-Historie, wird NICHT verändert).
- `v0424-targeted.yml`: einziger Job `recovery`, matrix docker+podman, `if:` verlangt `[demo-runtime]`/`[demo-podman-runtime]`/`[demo-docker-runtime]` im **head commit message** des Push-Events (Zeile 12). Pin-Commit "pin core C2-final 1861ed07" traegt keinen Marker → Erwartung: Run startet, Job SKIPPED. BEFUNDE 1.
- PR #119: OPEN, head `f4c080698bfa210a4f1903579f9988911b375ee5`, 0 checks.

## BEFUNDE

1. (erwartet, wird im Lauf verifiziert) Ohne Marker im head-commit-message wird der `recovery`-Job skipped — der Run "entscheidet" kein Gate. Minimal-Korrektur gemaess Repo-Konvention (a0d0be8 "re-trigger: ... [demo-runtime]" als Präzedenzfall): normaler push eines Re-trigger-Commits mit Marker, KEIN force, erst nach dokumentiertem Skip-Befund.

## EXECUTION LOG

(wird gefüllt)
