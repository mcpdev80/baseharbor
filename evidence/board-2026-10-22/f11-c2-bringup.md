# f11-c2-bringup — C2BRINGUP lane evidence

Mission (delegation-inbox line C2BRINGUP / prompts/f11-c2-bringup.txt):
sync fix/edit-refusal-preview-line (PR #1038) and fix/v0426-stepcap-split (PR #1039)
onto develop 281d1ae0 via normal merge commits (no force), push, watch checks to
terminal, report per-PR check tables + first fail error. No merges, no other branches.

## PLAN
- [x] 1. Fetch origin in baseharbor; verify develop contains 281d1ae0; record branch HEADs
- [x] 2. Merge develop 281d1ae0 into fix/edit-refusal-preview-line (normal merge commit), push
- [x] 3. Merge develop 281d1ae0 into fix/v0426-stepcap-split (normal merge commit), push
- [x] 4. Watch PR #1038 checks to terminal
- [x] 5. Watch PR #1039 checks to terminal
- [x] 6. Report final check tables + first-error lines for any fail
- [x] 7. ## repo-rule compliance + END F11-BRINGUP-DONE

## Log
- 2026-10-23 fetch OK; origin/develop = 281d1ae0 verified. PR #1038 head c37933d9, PR #1039 head 18735dfc, both OPEN, worktrees clean.
- #1038: merge commit 1192004d (parents c37933d9 + 281d1ae0), no conflicts; pushed c37933d9..1192004d (normal, no force) via worktrees/f7-t1-edit-preview.
- #1039: merge commit 940fa694 (parents 18735dfc + 281d1ae0), no conflicts; pushed 18735dfc..940fa694 (normal, no force) from baseharbor main tree.
- Watching checks to terminal.

- Checks terminal: #1038 run 39866635495 completed SUCCESS; #1039 produced zero check runs (see finding).

## Final check tables
### PR #1038 (head 1192004d, run 39866635495) — 10 pass / 10 skip / 0 fail / 0 pending
| Check | Result |
|---|---|
| Workflow policy | pass |
| PR · Trusted Plan | pass |
| PR Selected Checks | pass |
| PR · Focused Python | pass |
| PR · Validation · cli-gofmt | pass |
| PR · Validation · cli-build | pass |
| PR · Validation · cli-vet | pass |
| PR · Validation · source-contracts | pass |
| PR · Validation · cli-guided-tests | pass |
| PR · Validation · wizard-app-edit-tests | pass |
| Consumer Pins / Demo Pin / Exact Candidate / Public Contracts / Runner Interface / Selected Runtime / validate / Targeted HA docker / Targeted HA podman / Release Source Preflight | skip (path filters) |

### PR #1039 (head 940fa694) — 0 pass / 0 skip / 0 pending; NO check runs ever created
| Workflow run | Event | Result |
|---|---|---|
| .github/workflows/ci.yml (39866652511) | pull_request | completed/failure, 0s, 0 jobs — "workflow file issue" |
| .github/workflows/pre-release-runtime-suite.yml (39866644958) | push | completed/failure, 0s, 0 jobs — "workflow file issue" |

### First error / root cause (pre-existing, NOT introduced by this sync)
- Introduced by PR-head commit 18735dfc in .github/workflows/pre-release-runtime-suite.yml:198-208:
  `timeout-minutes: ${{` opened as multi-line plain scalar; closing `}}` sits at indent 8 (same as the key),
  so the scalar ends at line 200 and `}}` becomes a stray token.
- First parse error: `expected <block end>, but found '}'` at the `}}` line (reproduced locally with
  `python3 -c "import yaml;yaml.safe_load(...)"` on blob 940fa694:path).
- Confirmed pre-existing: head~1 2adae47a produced normal check runs; 18735dfc (20:56Z push, 21:01Z PR run)
  already failed at 0s before C2BRINGUP. ci.yml fails only because ci.yml:768 reuses the broken file from PR head.
- Fix recipe (for the branch owner, NOT applied here — out of C2BRINGUP scope): keep the expression on one
  line or quote/fold it as a literal block so `}}` is not dedented to key level, e.g. revert to the
  single-line `${{ ... && 25 || ... }}` form.
- Consequence: #1039 cannot show PR checks until that commit is corrected; syncing alone can never turn them green.

## repo-rule compliance
- Isolation: merges done only in the branch's own checkouts (worktrees/f7-t1-edit-preview; main tree while it
  still had a clean checkout of the branch); another lane reused the main tree at 00:43 — verified after-the-fact
  that origin refs were already pushed and intact (no lost work).
- No force-push, no merges of PRs, normal merge commits only (1192004d, 940fa694), no product changes,
  no conflict resolution needed, no other branches touched.
- CI triggers were exactly the two branch syncs; no release matrix forced. Docker/Podman targeted gates were
  path-filtered to skip on #1038 (not serialized).
- Finding reported with file:line instead of silent out-of-scope fix, per AGENTS.md conflict rule.

END F11-BRINGUP-DONE
