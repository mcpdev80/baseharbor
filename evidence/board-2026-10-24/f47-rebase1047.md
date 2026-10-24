# f47 — Rebase #1047 (gate-plan apptest lane), Beleg ohne Merge

Mission: ops/missions/f47.txt. PR #1047 (branch fix/f27-gateplan-apptest-lane, worktree baseharbor/worktrees/f27-gateplan-lane, head 79e0aa2b) ist ACCEPT-reviewt, Checks waren grün (run 39998286306), aber mergeStateStatus=BEHIND (3 develop-Commits voraus), Ruleset verlangt strikte Freshness.
Grenzen: git NUR in /home/marcel/dev/baseharbor-ecosystem/baseharbor (pwd vor JEDEM git), KEIN git im Root-Repo, KEIN gh pr merge, KEIN gh pr review, KEIN force-push, KEIN --admin, KEINE Änderung an develop/main/fremden Branches.

## PLAN

- [x] 0. Evidence + native Todos (diese Datei)
- [x] 1. Realitätscheck: fetch origin, develop-Tip, merge-base, Worktree-Zustand f27-gateplan-lane, Diff-Umfang des PR
- [x] 2. Branch auf develop-Basis gebracht (Merge statt Rebase — Begründung im Protokoll; kein Konflikt, TRUSTED-Regel greift inhaltlich)
- [x] 3. Lokale minimum Pruefung: repo-ueblicher Runner (unittest-Kette 98+8+7+5, OK); kein --selftest im Script; keine Docker/VM/Full-Matrix
- [x] 4. Push origin/fix/f27-gateplan-apptest-lane als Fast-forward done (79e0aa2b..40351fd3, KEIN force-push, KEIN --admin)
- [x] 5. gh pr checks 1047: new head 40351fd3, run 40076426268 SUCCESS (~8 min, Guard 20 min gehalten)
- [x] 6. Evidence-Vollständigkeit: siehe Protokoll (PR-URL, base, head, Diff, Test-Auszug, Run-IDs, mergeStateStatus)
- [x] 7. END F47-REBASE1047-DONE (neue head-SHA unten)

## Regelkonflikt-Vormerkung

Mission Schritt 1 sagt "Rebase", Schritt 4 + STOP-Kriterium verbieten force-push. Ein history-rewriting rebase ist auf einem bestehenden PR-Branch nur mit force pushbar. Auflösung wird vor dem Push dokumentiert (AGENTS.md Konfliktregel); vorgezogen wird der Pfad, der alle STOP-Kriterien wahrt.

## Protokoll

### Schritt 1 — Realitätscheck (08:00 UTC)

- origin/develop = **a2abc17a** ("ci(gate-plan): bootstrap-eligible exception … (#1059)"); voraus seit merge-base **1861ed07** genau 3 Commits: 79e64084 (#1053), 24c33afd (#1058), a2abc17a (#1059).
- PR https://github.com/mcpdev80/baseharbor/pull/1047: base develop, head 79e0aa2b (1 Commit), 72 insertions, Diff = `scripts/pr-gate-plan.py` + `scripts/test_pr_gate_plan.py` — exakt wie Mission.
- Mission-Worktree-Pfad "baseharbor/worktrees/f27-gateplan-lane" existiert nicht dort; real: `/home/marcel/dev/baseharbor-ecosystem/worktrees/f27-gateplan-lane` (sauber, auf Branch fix/f27-gateplan-apptest-lane @79e0aa2b, tracking ok). Laut `git worktree list` der baseharbor-Repos — Nutzung gemäß Mission, keine fremden Arbeitsstände (dirty=0).

### Schritt 2 — Regelkonflikt-Auflösung: Merge statt Rebase (dokumentiert, nicht stillschweigend)

- Konflikt: Schritt 1 sagt "Rebase", Schritt 4 + STOP-Kriterium verbieten force-push. Ein history-rewriting rebase von 79e0aa2b auf a2abc17a wäre auf dem PR-Branch nur mit force pushbar (nicht-fast-forward). STOP-Kriterien sind hart -> **git merge origin/develop in den PR-Branch** erreicht dasselbe Ziel (neuer head enthält develop-Tip als Ancestor -> BEHIND aufgelöst, strikte Freshness wiederhergestellt), Push bleibt fast-forward. Squash-Merge-Policy der Codex-Lane bleibt unberührt (Merge-Method der PR-History ist für den Squash irrelevant).
- Overlap-Analyse (Zeile für Zeile): Beide Seiten ändern `scripts/pr-gate-plan.py`, aber disjunkte Regionen:
  - PR-Seite: +APPID_ENV_ROBUST_TEST_REVIEWED_HEAD/FILES Konstantenblock hinter Zeile ~142 (#1046 Follow-up, exakte 6 Test-Dateien) und +qualifying-Zweig in `plan()` bei ~993 (`core752_reviewed_merge_chain(...APPID_ENV_ROBUST_TEST_REVIEWED_HEAD)`, require(CODE_CHECKS)).
  - develop-Seite (#1059): +`bootstrap_eligible()` bei ~678 und 2-Zeilen-Änderung `binding_checks = trusted_binding_checks(...) or bootstrap_eligible(...)` bei ~1086.
  - Kein Overlap -> `git merge --no-edit origin/develop` ohne Textkonflikt (Merge-Commit 40351fd3). TRUSTED-Regel greift inhaltlich automatisch: develop-Akte (bootstrap_eligible) bleibt unverändert erhalten, PR-lane bleibt als eigener Zweig davor; keine "ours/theirs"-Raten erforderlich.

### Schritt 3 — Lokale Minimum-Prüfung (im Worktree, HEAD=40351fd3)

- `python3 -m pytest` nicht installiert -> repo-ueblicher Runner gemäß CI (`ci.yml:640` + `:714`): `python3 scripts/test_pr_gate_plan.py` (+ `-m unittest scripts/test_pr_gate_plan.py`, identisches Ergebnis).
- Ausgabe (vereint): `Ran 98 … OK`, `Ran 8 … OK`, `Ran 7 … OK`, `Ran 5 … OK` — 118 Tests, 0 FAIL/0 ERROR. (Die Zahl 98 = 93 Basis + 5 neue develop-Bootstrap-Tests + PR-eigene; OK.)
- `scripts/pr-gate-plan.py --selftest`: existiert nicht (kein selftest-Handler im Script) — gemäß Mission "falls vorhanden" entfallen.
- Merge-Inhalt verifiziert: APPID_ENV_ROBUST-Block 2 Treffer + `def bootstrap_eligible` 1 Treffer im gemergten Script.

### Schritt 4 — Push

- `git push origin fix/f27-gateplan-apptest-lane` -> `79e0aa2b..40351fd3` (fast-forward, kein force, kein --admin). Remote == local HEAD = 40351fd3.

### Schritt 5 — Checks auf neuem head (Guard ab 08:01:40Z, Limit 20 min)

- Run **40076426268** "CI · PR #1047" @ head 40351fd3, created 08:01:40Z, conclusion **success** — "PR Selected Checks" pass (job 120820955384), Trusted Plan pass, Workflow policy pass, Focused Python/Validation pass; 7 SUCCESS + 10 SKIPPED, keine pending, keine failure um 08:07:44Z (≈ 6 min nach Start, Guard deutlich eingehalten, kein Rerun, keine Labels).

### Schritt 6 — Merge-Zustand (exakte Feststellung)

- `gh pr view 1047 --json mergeStateStatus,mergeable,headRefOid` (08:07Z): head=40351fd30af9d1b12a3ea5b4b450056ea714f475, mergeable=**MERGEABLE**, mergeStateStatus=**BLOCKED — NICHT CLEAN**.
- Ursache: BEHIND ist aufgelöst; BLOCKED bleibt, weil das develop-Ruleset 23754541 (approving review + code-owner + last-push-approval; dismiss_stale_reviews_on_push) auf dem neuen head GitHub-Reviews verlangt. Review-Abgabe ist Codex-Lane (Mission Grenze) — hier NICHT ausgeführt.

## Abnahme-Kriterien

- neuer head auf develop-Basis gepusht: JA (40351fd3, FF)
- lokale Planner-Tests grün: JA (118/118 OK, repo-Runner)
- Checks grün: JA (run 40076426268 success, Selected Checks pass)
- mergeStateStatus dokumentiert: JA (BLOCKED — Review-Gate, s.o.)

END F47-REBASE1047-DONE head=40351fd30af9d1b12a3ea5b4b450056ea714f475 base=a2abc17a run=40076426268 mergeStateStatus=BLOCKED(review-gate@Codex-Lane)
