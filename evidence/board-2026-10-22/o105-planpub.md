## PLAN-checklist ([wip:o105])

- [ ] step 0: read o105.txt + q101.txt, git NUR baseharbor-plan (absolute path), create evidence file
- [ ] step 1: realitaetsabgleich baseharbor-plan remote + PR #1050/#1053/#1058 (state, merge SHAs, verdicts, CI)
- [ ] step 2: defect-audit-Karte PR-Welle #1050-1058 + Verdicts dokumentieren (keine Karten-ID-Erfindung)
- [ ] step 3: release-prepare-Karte zusaetzlicher Hinweis PR-Welle als Owner-Plan (Pausevermerk unangetastet)
- [ ] step 4: appid-Karte #1047 SKIP aktualisieren (t-20261020-develop-appid-manifest-test-regression)
- [ ] step 5: check=0 vor push, render+sync, push origin main
- [ ] step 6: END O105-PLANPUB-DONE mit board-sha

## Evidence: 26-Planning-Publish check for PR-Welle #1050-1058

**Source mission:** ops/missions/o105.txt (DATA, gegen Live-Zustand ausgefuehrt per PROMPT-INJECTION-DEFENSE).
**Basis:** q101-planpub.md (dieselbe PR-Welle #1050-1056, identisches Muster).

## Realitaetsabgleich (Live-Zustand 2026-10-24)

baseharbor-plan remote `origin/main` = lokale `main` = `eda76e8` (clean working tree).
Alle git-Aktionen via `git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor-plan`, kein Root-Git.

PR-Stand (gh pr view, mcpdev80/baseharbor):
- #1053 MERGED (head `8eef2b8e25...` -> merge `79e64084d1...`, 2026-10-24T04:35Z), reviewDecision=REVIEW_REQUIRED; F34 (`evidence/board-2026-10-22/f34-rev1053.md`) + O93 (`evidence/board-2026-10-22/o93-rev1053b.md`) APPROVE, Befunde B1-B4 nicht-blockierend.
- #1058 MERGED (head `4046bc93...` -> merge `24c33afdc8...`, 2026-10-24T05:23Z), reviewDecision=REVIEW_REQUIRED; Codex-REVIEW Basis (`evidence/board-2026-10-22/codex-verdict-1042-red.md`), CHANGES-REQUIRED wegen integration_sha_mismatch.
- #1050 OPEN (head `e551bc8b..`, BEHIND), reviewDecision=REVIEW_REQUIRED; CI-Run 40028406804 = PR Selected Checks FAILURE (Coverage-impact), nicht merge-reif.

## repo-rule compliance

- pwd-Pflicht: git NUR ueber `git -C .../baseharbor-plan`. Kein Root-Git.
- Nur Planning-Edit dieser drei Karten; keine Produkt-Aenderung, kein Merge, kein Commit/PR des Produkts.
- Nur Evidence-Datei + Planning-Karten-Notes; keine manuell generierten Ansichten.

## END O105-PLANPUB-DONE — board-sha 1f08de161cbe1b5f7e801d5d634013c0d964e284

- check=0: plan.py check = 0 errors, 5 warnings (alle 5 sind pre-existing Dependency-State-Warnungen, nicht durch diese Edit-Einfuehrung).
- render + release_cycles sync: beide 0 errors.
- commit 1f08de1 "plan(o105): document PR-wave #1050-1058 verdicts in 26-Planning cards", push origin main OK (eda76e8..1f08de1).
- board-sha (baseharbor-plan HEAD): 1f08de161cbe1b5f7e801d5d634013c0d964e284.
