# F4 — READ-ONLY design review: Demo CI trigger plan (maintainer decision prep)

Recorded 2026-10-22T14:38Z UTC. Reviewer: coordinator session, model `agentgateway/qwen3.8-flash-next-spark`.
Subjects: baseharbor-demo PR #116 (`integrated/v0426-new-edit-native` @ `a9c6f0d5be4e3addbabd58345313cb75c97a613f`,
tree `c18ef9cf045630567252c3d167a9615c022495cf`), demo `.github/workflows/v0424-targeted.yml` (workflow id
379825635, active), core `.github/workflows/targeted-demo-acceptance.yml` + `pre-release-runtime-suite.yml`.
Purely read-only: `git fetch`/`git show`/`git ls-remote` and `gh api` GETs only; no dispatch, push, PR or file
mutation in any product or planning repo.

## Verdict (first)

**Recommend Option A — the documented push trigger on `work/v0.4.26-ecosystem-release`; reject Option B
(workflow_dispatch+ref edit).** No workflow edit is needed for ANY of the F3 §4 native gates, because:

1. The demo `v0424-targeted.yml` push trigger already runs the backup-restore/recovery gate on Docker+Podman
   and fires on the **pushed commit's own workflow version** (GitHub docs: the push event "includes workflows
   that are not merged into the default branch"). PR #116 already adds `work/v0.4.26-ecosystem-release` to the
   glob, so after #116 is merged the branch-creation push works off trusted main-lineage policy — zero extra
   CI-policy change.
2. The guided-light, heavy-data and MCP gates are **not in the demo workflow at all** (it only runs
   `scripts/acceptance.sh backup-restore`, plus static fixture-consent checks). They are executable **today,
   with no edits anywhere**, via the core repo's active **Targeted Atomic Gate** (`targeted-demo-acceptance.yml`,
   `workflow_dispatch` inputs `ref`/`demo_ref`/`runtime`/`gate`; the trusted workflow definition comes from
   `develop`, the code under test from the given `demo_ref`) — exactly the #1030-consistent trust model.
3. Option B would merge a CI-policy change into demo `main` (dispatch only fires when the workflow file is on
   the default branch) to create a dispatch surface that **already exists in the core repo** — permanent extra
   surface, an additional review round, and no added coverage. Strictly worse.

**Corrections to `o5-demo-pr.md` (board hygiene):** (i) `origin/main`'s trigger list is
`[work/v0.4.24-ecosystem-release, work/v0.4.25-ecosystem-release]` only — the `.26` entry exists solely in the
PR-branch file; (ii) `work/v0.4.26-ecosystem-release` does not exist remotely yet; (iii) the heavy-data/MCP
journeys do **not** "share this same workflow file" — they run via the core Targeted Atomic Gate.
Precondition unchanged: Core re-freeze to the final immutable v0.4.26 SHA in `baseharbor-core.ref` via a
qualified normal merge **before** any run counts as v0.4.26 native evidence (F3 §4.1; HANDOFF freeze order).

## (a) Exact triggers available today, WITHOUT any workflow edit

**baseharbor-demo** — only active workflow `v0424-targeted.yml` (others: disabled_manually, not on main):
- `on.push.branches`: `work/v0.4.24-ecosystem-release`, `work/v0.4.25-ecosystem-release` (main file);
  `+ work/v0.4.26-ecosystem-release` (PR #116 file — one-line glob diff, blob `a24c855`, verified).
- No `tags:` filter → tag pushes never trigger. No `paths:`. No `workflow_dispatch` (404 on dispatch, per O5).
- Job `if:` on `github.event.head_commit.message`: `[demo-runtime]` → matrix `["docker","podman"]`;
  `[demo-docker-runtime]` → docker only; `[demo-podman-runtime]` → podman only;
  `[demo-fresh-recovery]` → `BASEHARBOR_DEMO_RECOVERY_MODE=fresh-owner` else `retained`.
- Executes: static consent/syntax checks + `scripts/acceptance.sh backup-restore` (atomic mode,
  `DEMO_ATOMIC_GATE=1`), Core built from the pushed commit's `baseharbor-core.ref` (must be 40-hex).
- Concurrency `demo-v024-<ref>`, cancel-in-progress false → same-branch pushes serialize, never cancel.

**baseharbor (core)** — demo-relevant routes:
- `targeted-demo-acceptance.yml` (active): `workflow_dispatch` inputs `ref` (Core), `demo_ref` (exact demo
  commit), `runtime` ∈ static/docker/podman, `gate` ∈ demo `tests/gate-resources.json` (`guided`,
  `data-capabilities`, `mcp`, `backup-restore`, …) → calls `pre-release-runtime-suite.yml`; demo scripts
  cloned at `demo_ref`; runtime step timeouts 8 min (other gates), 20 min (recovery profile), 40 min
  (backup-restore); success evidence artifact `pre-release-gate-<runtime>-<gate>-evidence-<run>-<attempt>`
  (30-day retention). Push route `runtime-validation/targeted/<runtime>/<gate>` resolves `demo_ref` only for
  CHANGELOG 0.4.24 (ref lookup), 0.4.25 (pin script) or via `docs/releases/v<version>.demo-ref` — **no
  `v0.4.26.demo-ref` exists on develop → push route NOT usable for .26 yet; dispatch IS**.
  Concurrency group `baseharbor-release-qualification` (all such runs serialize; no cancel).
- `ci.yml`: push only on `runtime-validation/ci/**`, `.../source-preflight/**`, `.../ha/**` + PRs to
  develop/main; `pre-release.yml`: dispatch-only full suite.

**Throwaway refs from the PR head — qualification test:** push events read the workflow file and the
`on.push.branches` filter **from the pushed commit**, so content is sufficient (the PR branch contains the
needed workflow version — verified). But the **ref name must match** one of the three literal branch names;
no other temp branch name, no tag, and no `refs/pull/*` ref qualifies. Mechanically, creating
`work/v0.4.26-ecosystem-release` **directly from unmerged `a9c6f0d5`** with a `[demo-runtime]` commit would
fire — but that executes the PR's (unqualified) workflow policy; per the HANDOFF principle it may serve as a
diagnostic at most, **never as v0.4.26 release evidence**. Qualified sequence = branch created from post-merge
main (workflow file then main-lineage, i.e. reviewed policy) + Core re-pin commit carrying the tag.

## (b) Cost estimate per option

| Aspect | Option A (push trigger) | Option B (dispatch+ref edit) |
| --- | --- | --- |
| Workflow edits | **None** (the needed edit rides #116, already reviewed by F3) | Demo CI-policy PR: author, independent review, merge (≈ #1030 detour) |
| Runner minutes | Demo: 2 jobs × ≤60 min cap (~20–40 min real). Core dispatches: 5 gate runs × ~15–30 min, serialized by `baseharbor-release-qualification`. Total ≈ 1.5–4 h wall, ≈ 2.5–6 runner-hours. Public repos → GitHub-hosted minutes uncharged; cost is wall-clock/queue | Same runtimes **plus** future ad-hoc dispatch exposure (permanent surface, no ref-name gate) |
| Shared-stack risk | None external: isolated `$RUNNER_TEMP` fixture dirs, `localhost/baseharbor-runtime:demo-v024` / `:pre-release` tags only, `persist-credentials:false`, `permissions: contents: read`; no registry/deploy writes | Same per run, but the merged dispatch surface lets later candidates be run without maintainer-authored pushes — the exact drift the P1/trusted-base finding guards |
| Cleanup | Keep `work/v0.4.26-ecosystem-release` as the release record (`.24`/`.25` kept, `.25` in main history); artifacts age out (30 d gate evidence, default demo retention) | Nothing removable: policy change is permanent |
| Fit with HANDOFF freeze order | Direct match (".25 branch = `chore(release): pin …` commits"); Core re-pin lands as the tagged head commit | Duplicates the core repo's already-trusted dispatch route; adds a second authority |

## (c) Maintainer execution plan (literal commands)

Prereq C = final immutable v0.4.26 Core SHA (develop, after the #1030-accepted lineage + source integrations).

```bash
# 1) demo: Core re-pin as a normal reviewed PR (freeze order)
cd baseharbor-demo && git fetch origin
git switch -c chore/v0426-core-pin origin/main
printf '%s\n' "$C" > baseharbor-core.ref
git commit -am "chore(release): pin final immutable v0.4.26 Core"
git push -u origin chore/v0426-core-pin && gh pr create --base main --fill
# independent review, then normal merge; #116 merges to main before/with this

git fetch origin
# 2) qualified release-branch push (fires "Targeted Demo compatibility" on Docker AND Podman)
git switch -c work/v0.4.26-ecosystem-release origin/main
git commit --allow-empty -m "ci(demo): qualify v0.4.26 backup-restore [demo-runtime]"
git push -u origin work/v0.4.26-ecosystem-release
# per-runtime retries: "[demo-docker-runtime]" / "[demo-podman-runtime]"; fresh-owner mode: add "[demo-fresh-recovery]"

# 3) guided-light + heavy-data + MCP gates via the CORE repo's existing dispatch (no edits anywhere)
D="$(git rev-parse origin/main)"   # demo main including #116 + pin
for spec in static/mcp docker/guided podman/guided docker/data-capabilities podman/data-capabilities; do
  gh workflow run targeted-demo-acceptance.yml --repo mcpdev80/baseharbor --ref develop \
    -f ref="$C" -f demo_ref="$D" -f runtime="${spec%%/*}" -f gate="${spec##*/}"
  sleep 2   # runs serialize on concurrency group baseharbor-release-qualification
done
gh run list --repo mcpdev80/baseharbor --workflow "Targeted Atomic Gate" --limit 6
```

Expected runs: in **baseharbor-demo Actions** — workflow "Targeted Demo compatibility", run title = the push
commit message, jobs "Demo install and backup recovery · docker" and "· podman" (ubuntu-latest /
ubuntu-26.04), artifacts `demo-v024-docker-<run_id>` / `demo-v024-podman-<run_id>`
(results.tsv, report.md, native-topology-*.json, recovery-qualified.json). In **baseharbor Actions** —
"Targeted Atomic Gate" run-name "targeted · develop", one job "<runtime> · <gate>" per run, artifact
`pre-release-gate-<runtime>-<gate>-evidence-<run_id>-<attempt>` with the live-gate receipts
(`new-edit-zero.json` etc. flipping `native_runtime_qualified=true` — F3 §4.6).

Evidence to file (Evidence-Disziplin): board evidence file with before/after sha256 of the candidate heads
(`a9c6f0d5`/`D`/branch tips) and C; every run URL + conclusion + attempt; downloaded receipts' sha256;
runner-fit observation for the data-capabilities container inventory vs the 8-min runtime step recorded
BEFORE the remaining dispatches (F3 §4.5); planning card/handoff updated to `review`→`closed` per status
changes. Failures keep the 1-day `demo-failure-*` / `atomic-failure-*` diagnostics artifacts.

## repo-rule compliance

- Read baseharbor-demo `AGENTS.md` + `CONTRIBUTING.md`, baseharbor `AGENTS.md` +
  `docs/DEVELOPMENT_GUIDELINES.md` (line 125 review/release boundary, CI files under code-owner review),
  ecosystem `AGENTS.md`, HANDOFF demo section (lines 77–89), `o5-demo-pr.md`, `o2-demo-integrate.md`,
  `f3-demo-review.md`, `q3-1030-dossier.md`, `codex-merge-1030.md` and the trusted-base P1 finding
  (`evidence/board-2026-10-20/codex-verdict-podman-pair.md`).
- Live facts verified 2026-10-22 after fetch: demo rulesets `[]`, demo `main` unprotected, all merge methods
  allowed, no CODEOWNERS in demo; core develop rulesets 23754541 (PR+linear-history+non-ff+deletion,
  mcpdev80 PR bypass) and 25223521 (`PR Selected Checks`, no bypass) — Option A touches no protected ref
  (trigger push targets an unprotected demo work branch; the core route is `workflow_dispatch`).
- "Do not execute unqualified PR policy": honored — recommendation only executes merged, reviewed workflow
  definitions (demo main post-#116; core develop dispatch file); the pre-merge throwaway variant is
  explicitly flagged as diagnostic-only.
- Read-only: no product/planning file outside this board evidence dir was written; no GitHub state mutation
  (only GET APIs, `ls-remote` and fetches); candidate trees re-verified against F3 hashes.

F4-TRIGGERPLAN-DONE
