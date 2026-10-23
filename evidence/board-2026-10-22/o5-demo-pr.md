# O5 — Demo PR lane: baseharbor-demo `integrated/v0426-new-edit-native`

Recorded 2026-10-22 UTC. Coordinator: Flash. Repo: `baseharbor-demo` (product).
Branch: `integrated/v0426-new-edit-native` @ `a9c6f0d5be4e3addbabd58345313cb75c97a613f`.

## Step 1 — remote fetch + head + main move check

- `git fetch origin` clean.
- `origin/main` == `a92347900ed4d1015d9bcde3d265cf9326664a40` — **matches the expected base, main has not moved.**
- `origin/integrated/v0426-new-edit-native` == local `a9c6f0d5` — **branch already up-to-date, working tree clean.**
- No rebase required (main unchanged, branch head identical to remote). The 41 local unit tests were
  executed green in the prior lane (`o2-demo-integrate.md`); nothing to re-run.

## Step 2 — PR opened

- **PR #116**: `https://github.com/mcpdev80/baseharbor-demo/pull/116`
  `integrated/v0426-new-edit-native` → `main`. State OPEN, 15 files changed, +1719 / −157, no merge.
- Body carries: handoff provenance (combined patch `a7a4a3a` / SQL patch `a9c6f0d`), patch sha256 refs
  (full 64-hex from `patch-inventory.json`), full test inventory (41 unit incl. the
  `test_canonical_postgres_layout_excludes_same_image_admin_and_init` regression), and an honest scope
  section enumerating the still-outstanding native journeys (Core re-freeze + guided-light + heavy-data
  + MCP gate + runner fit).
- No merge performed (per task).

## Step 3 — available CI inspection (`gh api repos/.../actions/workflows`)

Four workflows exist in the repo:

| id | name | state | trigger on main/release branches |
| --- | --- | --- | --- |
| **379825635** | **Targeted Demo compatibility** (`.github/workflows/v0424-targeted.yml`) | **active** | **`push` only**, branches `[work/v0.4.24-ecosystem-release, work/v0.4.25-ecosystem-release, work/v0.4.26-ecosystem-release]` |
| 365682246 | init-targeted | disabled_manually | not present on main/release (feature-branch only) |
| 365686322 | recovery-targeted | disabled_manually | not present on main/release (feature-branch only) |
| 365851868 | Acceptance Harness Check | disabled_manually | not present on main/release (feature-branch only) |

**Only `v0424-targeted.yml` (id 379825635) is active on main/release branches.** Its matrix job
(`recovery`) is additionally commit-message gated:
`if: contains(head_commit.message, '[demo-runtime]') || [demo-podman-runtime] || [demo-docker-runtime]`.
It builds the pinned Core ref (`baseharbor-core.ref` = `f325b93`, the v0.4.25 pin), prepares an
isolated rootless runtime, installs the exact pinned Core binary, and runs
`scripts/acceptance.sh backup-restore` on a chosen `runtime` (docker / podman).

### Dispatchability of the light guided gate

- **Not dispatchable.** The workflow declares **no `workflow_dispatch`** (confirmed via the workflow
  metadata endpoint — no dispatch field — and the `POST …/actions/workflows/379825635/dispatch`
  endpoint returning **HTTP 404**). There is **no `ref` input** to supply.
- The only way it fires is a **push** to one of the three `work/v0.4.2X-ecosystem-release` branches
  **with a commit message containing a `[demo-*]` runtime tag**. A PR merge into `main`, a push to
  `integrated/v0426-new-edit-native`, or a dispatch call **cannot** run it.
- `gh pr checks 116` → **no checks reported** on the branch, as expected (push-only, commit-gated).

### Missing dispatch input for the maintainer merge decision

To run the light guided gate (and, per `o2-demo-integrate` §4 / F3 §4, the heavy-data + MCP journeys
that share this same workflow file), the maintainer must supply **both** of the following, which the
workflow exposes as **no inputs**:

1. **A target branch push**: push the candidate to `work/v0.4.26-ecosystem-release` (or re-target the
   workflow's `push.branches` to include the integration branch) — **or** add a
   `workflow_dispatch` block with a `ref` input to `.github/workflows/v0424-targeted.yml`.
2. **A commit message runtime tag** on that push: `[demo-runtime]`, `[demo-docker-runtime]`, or
   `[demo-podman-runtime]` (the matrix runtime selector reads these tags; without one the job's
   `if:` is false and nothing runs).

Neither is available from a PR-branch dispatch context. **The maintainer therefore owns the trigger:**
either push to a `work/…-ecosystem-release` branch with the tag, or wire `workflow_dispatch`. See
F3 §4 items 1–5 for the exact native-run obligations (Core re-freeze to the final immutable v0.4.26
SHA first, guided-light on Docker + rootless Podman, heavy-data SQL on both runtimes, MCP static gate,
runner resource fit for the added SQL/paused-sentinel containers).

## Step 4 — NOT merged

No merge, no force-push, no release action. Acceptance and the CI trigger remain with the Maintainer.
The PR is an integration candidate backed by a read-only APPROVE review (`f3-demo-review.md`), not
v0.4.26 native evidence.

## repo-rule compliance

- Read `baseharbor-demo/AGENTS.md` + `CONTRIBUTING.md` (planning authority in `baseharbor-plan`),
  `o2-demo-integrate.md`, the F3 review, and the handoff `HANDOFF.md` Demo section before acting.
- Central ownership respected: preserved `origin/main` (untouched), no claim taken, no existing
  branch/PR overwritten, no force-push, no CI dispatched, no merge.
- Evidence discipline: candidate head `a9c6f0d5` verified identical to `origin/integrated/…` after
  `git fetch`; patch sha256 refs copied verbatim from `patch-inventory.json`.
- No product files written; the only write this lane performed was the GitHub PR (#116) in this
  product repo, plus this evidence file in the coordinator board directory.

O5-DEMO-PR-DONE

(End of file — total lines omitted; coordinator board artifact)
