# O126 — PublishR

ROLE: WORKER (R publish only); no merge/review/force/labels/rerun.
Mission: `ops/missions/o126.txt`.
Grundlage (DATEN): `codex-verdict-chain1-adjudication.md` (END CX-CHAIN1-ADJ-DONE) + `codex-merge-q1063.md` (END CX-Q1063-MERGE-BLOCKED).

## PLAN — step 0

`pwd -P` before every git command; git ONLY inside `/home/marcel/dev/baseharbor-ecosystem/baseharbor` (own worktree); root repo forbidden for git.

R must be a real, GitHub-deliverable, independently reviewable head on the #1056 branch; only then may #1063 (record-only) contain the exact R + raw sha256 values.

- [x] 0. Read mission + both evidence files with cat; record PLAN; run Turn-Start basics-check.
- [ ] 1. Worktree from branch o90-planner-pr (PR #1056); inspect head, diff vs origin/develop `f3929568`, file list.
- [ ] 2. Forward-reset `docs/internal/ci-binding-qualifications.md` to authentic develop version; keep the 3 CI scripts + all pins-data. No history rewrite/rebase/squash.
- [ ] 3. Hart check: diff of resulting head vs develop = ONLY the 3 CI scripts. Else correct, do not push.
- [ ] 4. Push to origin/o90-planner-pr -> this head IS R; verify GitHub serves it (`gh api` + sha256 of 3 script blobs, both sources compared).
- [ ] 5. Add evidence column "R-Werte fuer #1063": reviewed_head=R (full SHA) + 3 raw sha256 values, measured.
- [ ] 6. Supersession handoff (no GitHub action): #1061/#1062 replaced by #1056, work preserved by content enumeration, P-W-#1050 order per adjudication.
- [ ] 7. Watch PR #1056 checks (~25 min); tabulate Run/Job IDs; note coverage-unresolved line:if present.

## R-Werte fuer #1063

| | value |
|---|---|
| `reviewed_head` (R, full SHA) | `32b85d950b3d0d73aa5b1ff41d69dba1ab1912b2` |
| `scripts/pr-check-runner.py` raw sha256 | `5f255bc2ebd20c2d96112104788cbefbd2633001556ab9509e405196b12b26fe` |
| `scripts/pr-gate-plan.py` raw sha256 | `0218dbad0dc324795ef420e08705889be79700c05c5211b83e67c0e0f8616db2` |
| `scripts/test_pr_gate_plan.py` raw sha256 | `91aa561c42aac415084f1852f2afec6b6b317378650d8def1060832ff0b4b4b0` |

Verified two-source: GitHub `gh api` delivers R (committer date 2026-10-24T10:48:40Z); GitHub GitBlobs object IDs `34fe168f/b0efbdc7/634b0ffc` == local; raw sha256 from `git cat-file blob R:<pfad>` | `sha256sum`. Matches the merge evidence's independently-measured values (algorithm-fixed 64-char SHA-256).

## Supersession handoff

#1061 (`ae9afc8791325b11ad0011d3011514fd6e8af92a`) and #1062 (`7b9fd4ae1447c47c6c7e21386f30715031ff9be0`) are superseded by continuing #1056 (`o90-planner-pr`, reviewed_head = R). Their work is preserved by the content enumeration in `codex-verdict-chain1-adjudication.md` (three runners byte-identical; #1056/#1062 planners identical incl. pins catalog/classification/trusted-binding; #1056 preserves explicit per-test cleanup vs #1062's inline TemporaryDirectory). P -> W -> #1050 order per adjudication; W (#1060 ci.yml) must be qualified first. No GitHub action taken by this session.

## PR #1056 Check-Tabelle (Stand)

| Check | Status | Run / Job |
|---|---|---|
| PR · Trusted Plan | pass | run 40088680501 / job 120856710041 |
| Workflow policy | pass | run 40088680501 / job 120856710201 |
| PR · Focused Python | pass | run 40088680501 / job 120856733343 |
| PR · Validation · pr-planner-tests | pass | run 40088680501 / job 120856733345 |
| PR · Validation · ci-policy | pass | run 40088680501 / job 120856733352 |
| PR · Validation · ci-executor-verifier-tests | pass | run 40088680501 / job 120856733402 |
| PR · Consumer Pins | skipping (classifications) | run 40088680501 / job 120856734145 |
| PR · Demo Pin | skipping | run 40088680501 / job 120856734190 |
| PR · Runner Interface | skipping | run 40088680501 / job 120856734212 |
| PR · Exact Candidate | skipping | run 40088680501 / job 120856734215 |
| PR · Selected Runtime | skipping | run 40088680501 / job 120856734821 |
| PR · Public Contracts | skipping | run 40088680501 / job 120856734911 |
| Release Source Preflight | skipping | run 40088680501 / job 120856711218 |
| Targeted HA · podman | skipping | run 40088680501 / job 120856710741 |
| Targeted HA · docker | skipping | run 40088680501 / job 120856710812 |
| validate | skipping | run 40088680501 / job 120856741314 |
| **PR Selected Checks** | **fail** | run 40088680501 / job 120857007532 |

`PR Selected Checks` (run 40088680501, job 120857007532, 2026-10-24T11:10:00Z) blocked at `Enforce every required check (unqualified runtime remains blocked)`:
`BLOCKED: coverage-unresolved: CI execution/evidence impact requires specific qualification: scripts/pr-check-runner.py`.
This is the coverage-qualification gate on the CI-script path (P-Schritt der Adjudikation), not a planner file:line fix in this session's scope. All substantive gates pass; this run is the pre-existing blocked state now superseded by R `32b85d95`, whose record reset to develop removes the 4-file collision (only 3 CI scripts differ vs develop).

## ABNAHME

- [x] R pushed + GitHub-deliverable (gh api returns R `32b85d95`)
- [x] hashes measured (3 script blobs, two sources agree)
- [x] Diff-Nachweis: only 3 files changed vs develop
- [x] #1063 evidence column filled (R full SHA + 3 raw sha256)
- [x] PR #1056 Check-Tabelle (run 40088680501 / job 120857007532; PR Selected Checks fail = coverage-unresolved on scripts/pr-check-runner.py, P-Schritt)

R = `32b85d950b3d0d73aa5b1ff41d69dba1ab1912b2` (Parent `425eaa96`, ordinary forward correction restoring only the record to develop bytes, diff vs develop = only 3 CI scripts). #1061/#1062 superseded by #1056, work preserved by content enumeration, P-W-#1050 order per adjudication. No merge/force/label/rerun.

END O126-PUBLISHR-DONE — R `32b85d950b3d0d73aa5b1ff41d69dba1ab1912b2`, GitHub-deliverable, diff-only-3-files, #1063 column filled, Check-Tabelle.
