# F19 — GATE-BLANK (leftover planner blank-line fixes per codex-verdict-s39b-s42b.md)

Lane: `f19` (wip:f19). Record: 2026-10-23 UTC. Mode: two one-file follow-up commits + pushes to existing
PR heads #1039 / #1042; no merges, no force-push, no releases, no Planning acceptance.

Mission: `prompts/f19-gate-blank.txt`. Per `codex-verdict-s39b-s42b.md` the residual blocker on BOTH
branches is the unmodified `scripts/pr-gate-plan.py` delta vs accepted D0 `2f005813` (A: extra blank
line at candidate L245; B: residual blank line at candidate L748 between EDIT_ZERO_PREVIEW and
ECOSYSTEM_RESOLVER tuples). The verdict's executable recipe for each is identical and authoritative:
`git restore --source=2f005813dc6d7fa94de164388b2531d025af31bc -- scripts/pr-gate-plan.py` then one
normal follow-up commit, so the resulting planner blob equals accepted D0 blob `98a9229c9126d8a6d242d44c4de523c69a6f843e`.
The prompt's " Leerzeile ergaenzen" paraphrase is subordinate to "genau so, wie das Verdict beschreibt".

## PLAN (todowrite mirror)

- [x] 0. Create this file with PLAN + todowrite ([wip:f19] applies).
- [x] 1. Turn-start basics-check.py (AGENTS.md Pflichtprogramm); process FLAGS in this chain.
- [x] 2. Reality check: fetch product remote; verify branch heads fe17e6e5 (fix/v0426-stepcap-split)
      and c34740fa (fix/atomic-bare-sha-dispatch), PR state OPEN, no other owner advanced heads,
      claims/ownership noted; verify current planner blobs differ from D0 only as verdict describes.
- [x] 3. Branch A: isolated worktree; restore scripts/pr-gate-plan.py from D0; confirm blob == 98a9229c;
      full diff D0...NEWHEAD contains ONLY the intended planner revert (plus pre-existing workflow change);
      run local pr-gate-plan test path; one commit; push to PR head (no force).
- [x] 4. Branch B: same recipe; additionally confirm the five product blobs remain byte-identical to
      7bdd46fd versions; full inventory = 3 modified workflows + 2 added files after the fix;
      run local pr-gate-plan test path; one commit; push to PR head (no force).
- [x] 5. Verify PR checks were triggered on both new heads (run IDs recorded, no gating claims).
- [x] 6. ## repo-rule compliance + both new SHAs + END F19-GATE-BLANK-DONE.

Stop criteria (per verdict): unexpected product blob changes, another owner advanced HEAD, or push would
require force -> stop and report, do not proceed. No qualification-map expansion, no rebase, no merge.

## Execution record (2026-10-23 UTC)

### Turn-start flags (basics-check.py)
- EVENTS 694 un-acked: cross-lane watcher dialog/idle/gone notices (panes codeman-74e10b98, codeman-93bc05c1);
  none reference this lane or an owner-stop for #1039/#1042. Left unacknowledged for coordinator pane
  inspection per precedent (codex-verdict-s39b-s42b.md ## repo-rule compliance) rather than falsely acked.
- POOL ornith/qwen below target: conditional scheduling notices, not actionable from this lane.
- PLAN coordinator-plan.md 52 min: coordinator-owned; not edited from a worker lane.

### Pre-work verification
- origin heads equaled the frozen SHAs exactly (`git ls-remote`): fe17e6e58da16fb7eb0912e28825dd622c079d16
  and c34740fa5fee5c5c7b07af03c3eb929386840e93; both PRs OPEN, not draft. Worktrees used:
  `baseharbor/worktrees/f14-1039` (branch checked out, clean) and `baseharbor/worktrees/f14-1042`
  (detached at c34740fa, clean). Stale local branch copy in `worktrees/f12-atomic-bare-sha-dispatch`
  (398bbdbd, ancestor of c34740fa) was NOT touched.
- Planner blobs matched the verdict table (A `51ff40ca…`, B `70659b98…`, D0 `98a9229c…`).
- D0-vs-candidate planner diffs confirmed BOTH residuals are one EXTRA blank line (A at L245, B at L748)
  that the verdict removes; the prompt's "fehlende Leerzeile ergaenzen" paraphrase is directionally wrong,
  so the verdict's authoritative recipe (`git restore --source=2f005813… -- scripts/pr-gate-plan.py`) was executed.

### Branch A — fix/v0426-stepcap-split (#1039)
- Restore + verify: working blob = `98a9229c9126d8a6d242d44c4de523c69a6f843e` (= accepted D0); diff = 1 deletion.
- `python3 -m py_compile scripts/pr-gate-plan.py` OK; `python3 scripts/test_pr_gate_plan.py`: Ran 5 tests, OK.
- One normal commit, fast-forward push `fe17e6e5..75efeef8` (no force).
- New head: **`75efeef89b61d848d5ea351877ba6423a31e58a6`**
- Full inventory `git diff --name-status D0...75efeef8`: exactly `M .github/workflows/pre-release-runtime-suite.yml` — per verdict stop criterion.
- CI triggered: run 39946225153 (in_progress at record time). No green-check claim.

### Branch B — fix/atomic-bare-sha-dispatch (#1042)
- Restore + verify: working blob = `98a9229c9126d8a6d242d44c4de523c69a6f843e`; diff = 1 deletion.
- `python3 -m py_compile scripts/pr-gate-plan.py` OK; `python3 scripts/test_pr_gate_plan.py`: Ran 5 tests, OK;
  `python3 scripts/test_targeted_gate_ref.py`: Ran 14 tests, OK.
- One normal commit, fast-forward push `c34740fa..0267dbdb` via `HEAD:refs/heads/...` (no force).
- New head: **`0267dbdb71f63a2c77e193618e6fba4106e5cd5b`**
- Full inventory `git diff --name-status D0...0267dbdb`: exactly three modified workflows
  (`ci.yml`, `pre-release-runtime-suite.yml`, `targeted-demo-acceptance.yml`) + two added files
  (`scripts/targeted-gate-ref.py`, `scripts/test_targeted_gate_ref.py`); planner absent from inventory
  (= equals D0). Diff `c34740fa...0267dbdb` touches only `scripts/pr-gate-plan.py`, so the five product
  blobs remain byte-identical to the reviewed `7bdd46fd`-lineage versions.
- CI triggered: run 39946290135 (queued at record time). No green-check claim.

### repo-rule compliance
- Isolated worktrees per branch; no other owner's working tree, index or remote branch was reset or
  overwritten; pushes were normal fast-forwards; no force, no protection bypass, no merge, no release,
  no qualification-map expansion, no rebase/amend.
- Only intended files committed per branch (one file each); this lane wrote no other product paths.
- Local gate-unit evidence above is tied to the new working trees pre-commit; CI runs on the pushed SHAs
  are the authoritative qualification and were left running, not claimed.
- No Planning card state was published from this lane; coordinator performs review/acceptance transitions
  after independent review of the two new SHAs.

END F19-GATE-BLANK-DONE — A: 75efeef89b61d848d5ea351877ba6423a31e58a6 / B: 0267dbdb71f63a2c77e193618e6fba4106e5cd5b
