# O33 — Atomic Acceptance Gate root-cause vs C2 (READ-ONLY)

Lane: `o33-atomic-gate-c2`
Recorded: 2026-10-23 UTC
Mode: **READ-ONLY**. Verifies, does not dispatch, does not modify, does not re-run.
Prompt: `prompts/o33-atomic-gate-c2.txt`

## 0) Question

o32 (`o32-defect-ledger-ci.md`) reported an **Atomic Acceptance Gate failure** at run
**39856259550** on develop head **2adae47a** (#1036 / pre-execution edit-result fix).
Determine whether **C2 = 2f005813dc6d7fa94de164388b2531d025af31bc**
(#1038 preview fix + #1040 gate qualification) resolves the **root cause** — judged
from code / workflow diffs, **no dispatch**.

Verdict options: `RESOLVED-BY-C2` / `STILL-BROKEN(recipe)` / `FLAKY`.

## 1) The failing run (re-read from live Actions API, read-only)

- `gh api .../actions/runs/39856259550`:
  - `name: "targeted · develop"`, `workflow_id 364538116` = `.github/workflows/targeted-demo-acceptance.yml`
  - `head_branch: develop`, `head_sha: 2adae47a`, `event: workflow_dispatch`, `conclusion: failure`.
- Jobs in the run (`.../runs/39856259550/jobs`, total_count 3):

  | job | conclusion | failed steps |
  |---|---|---|
  | `resolve` | success | — |
  | `selected_gate (podman, backup-restore) / podman · backup-restore` | **failure** | `Checkout BaseHarbor candidate`, `Show original assertion before cleanup`, `Cleanup runtime state`, `Generate atomic gate evidence`, `Upload atomic gate evidence` |
  | `Reference Journey ... Full E2E` | skipped | — |

- The cascade failures (`Show original assertion` / `Cleanup` / `Generate evidence` / `Upload evidence`)
  are all **post-conditions of the first real failure**, i.e. cleanup/diagnostic steps that have
  nothing to verify. The **single root-failing step is `Checkout BaseHarbor candidate`**.

## 2) Root cause of the failing step (from the step log)

`gh api .../jobs/120095197172/logs` for the podman backup-restore job:

- Job **Inputs**: `ref: 2adae47a`, `demo_ref: d9c78b57…`, `runtime: podman`, `gate: backup-restore`,
  `candidate_artifact: ""`.
- Failing step `Checkout BaseHarbor candidate` (step #2, conclusion **failure**, ran 39s) expands to
  `actions/checkout@3d3c42e5` with `ref: ${{ github.sha }}` — i.e. the checkout `ref` resolves to the
  **dispatch-time `ref` input**, which was literally **`2adae47a`** (not the develop HEAD at the time
  of the gate run).
- The checkout's fetch command (from the log):
  ```
  /usr/bin/git -c protocol.version=2 fetch --no-tags --prune --no-recurse-submodules --depth=1 \
     origin +refs/heads/2adae47a*:refs/remotes/origin/2adae47a* +refs/tags/2adae47a*:refs/tags/2adae47a*
  ```
  ran **3 times** (`Waiting 20/18 seconds before trying again`) and exited **1 each time**; final
  line: `##[error] The process '/usr/bin/git' failed with exit code 1`.
- **Interpretation:** the checkout asked GitHub for `refs/heads/2adae47a*` / `refs/tags/2adae47a*`.
  A **commit SHA is not a branch or a tag** — there is no `refs/heads/2adae47a*` or `refs/tags/2adae47a*`
  to fetch. The `checkout` action's ref spec only matches refs that *start* with the given string; a
  bare 40-hex SHA supplied as `ref:` produces a fetch refspec that matches nothing, so a single-sha
  checkout **cannot resolve** and the `git fetch` exits 1. This is a **workflow dispatch-time input /
  checkout-ref resolution defect**, not a product test failure.

## 3) What C2 actually changes (code diff, read-only)

- `gh api .../commits/2f005813…`: `num_files = 2`, parent chain `2f005813 → 281d1ae0` (the #1038/#1039
  sync base). C2 is a **single squash commit** of PR #1038 (author "Marcel Pfingstgräf").
- Files:
  - `cmd/baha/app_edit_apply_result.go` (+2/−4): removes the `if len(requests) == 0 { return zero plan }`
    bare-zero shortcut in
    `planApplicationProviderImpactForDesired` (the #1038 fix — the Edit-preview envelope defect).
  - `cmd/baha/app_edit_zero_preview_test.go` (+52/−0): the `wizard-app-edit-tests` regression.
- **No workflow file is in C2.** `.github/workflows/targeted-demo-acceptance.yml` is
  **byte-identical** between `2adae47a` and `2f005813` (`diff` clean; fetched from both via
  `…/contents/…?ref=`). The checkout / `resolve` / gate-input logic that produced the failing step is
  **unchanged by C2**.

## 4) Why C2 does not reach the failing step's root cause

- The failing step fails at **repo checkout of a bare SHA dispatch input**, ~18–39s into the job,
  **before any Go build, test, podman run, or the #1038 code path is even compiled**.
- C2 is a **product code fix** (Edit-preview envelope) plus, via #1040, the coverage-qualification
  binding that lets the `wizard-app-edit-tests` check execute. Neither touches checkout/ref-resolution
  nor the `ref=` dispatch input.
- C2 is not even reachable from `develop`: `develop` advanced past `7fbc1ec5` (the #1030 MCP-bootstrap
  merge) after the 2adae47a gate run. A fresh gate dispatch on develop fetches `origin +refs/heads/<develop-head>`
  — a valid branch ref — so this *specific* "checkout a bare commit SHA" failure mode is **not on the
  develop lineage at all**; it was an artifact of dispatching `targeted-demo-acceptance.yml` with a
  literal 40-hex `ref=` to a now-unreachable commit.

## 5) Verdict

### STILL-BROKEN(recipe)

- **Root cause = checkout of a bare commit SHA as the `ref:` dispatch input** in
  `targeted-demo-acceptance.yml` (podman backup-restore job, run 39856259550). The `git fetch
  --depth=1 origin +refs/heads/2adae47a*` refspec matches no branch/tag → git exit 1, step failure,
  cascade cleanup/evidence steps fail as post-conditions.
- **C2 does not resolve it:** C2 changes only two Go product files; the gate workflow (including the
  checkout ref logic) is byte-identical at C2. The failing code path is never compiled by C2.
- **FLAKY is disproven:** the failure is deterministic given a bare-SHA dispatch input (no race, no
  intermittent condition — a missing ref never resolves). It is a workflow-input defect, not a flake.
- This is a **verify-only lane (no dispatch)**; the recipe is documented below for the workflow owner.

### Recipe (for the branch / workflow owner — NOT applied here, out of scope and READ-ONLY)

- **Do not dispatch `targeted-demo-acceptance.yml` with `ref: <40-hex-sha>`** to `checkout`:
  checkout resolves `ref` against branch/tag refspecs, so a bare commit SHA cannot be fetched.
- Fix on the **dispatch side** (recommended): pass `ref: develop` (or `--ref refs/heads/develop`) and
  let the gate's own `resolve` step pin the candidate SHA internally (the workflow already has a
  `resolve` job that outputs a resolved candidate ref). If a bare candidate SHA must be checked out
  explicitly, resolve it to a branch ref or a full `refs/heads/<branch>` ref in the `resolve` step
  before the gate checkout, rather than feeding the raw SHA to `actions/checkout`.
- Secondary hardening (owner discretion): the `checkout` step should fail fast with a clear
  "candidate ref is not a branch/tag; resolve to a ref before checkout" message instead of three silent
  20/18s retry back-offs; and the `if-no-files-found: warn` artifact upload should be `fail`-or-
  skipped so a checkout failure is not masked as "no diagnostics uploaded".
- Stop criterion for re-test: re-run `targeted-demo-acceptance.yml` on `develop` HEAD with a valid
  branch `ref` and confirm the podman backup-restore job's `Checkout BaseHarbor candidate` step succeeds;
  then the #1038 code path (if covered by that gate) is what C2/#1040 actually governs.

## 6) Evidence of read-only method

- Live Actions API (`gh api`), authenticated as mcpdev80; no product repos cloned, no pushes, no
  dispatches. Workflow blobs fetched read-only via `…/contents/…?ref=…` at both `2adae47a` and
  `2f005813`; diff clean. Commit metadata fetched read-only via `…/commits/2f005813…`.

## PLAN — o33-atomic-gate-c2

1. **Scope (READ-ONLY):** re-read run 39856259550's failing job, root-cause the failing step, diff C2
   vs the failing run's head, conclude whether C2 resolves the root cause. No dispatch, no writes to
   products/CI/planning.
2. **Done when:** (a) failing job + failing step identified from the live log; (b) root cause stated
   with the exact failing command/refspec; (c) C2's actual changes enumerated and diffed vs the workflow;
   (d) verdict issued; (e) recipe recorded for the owner; (f) file written + `O33-ATOMIC-DONE`.
3. **Steps taken:**
   1. Read o32 (`o32-defect-ledger-ci.md`) + f11/f10/f9 for run/PR context.
   2. `gh api actions/runs/39856259550` → run summary, workflow `targeted-demo-acceptance.yml`,
      head_sha 2adae47a, conclusion failure.
   3. `gh api .../runs/39856259550/jobs` → identified the single root-failing step
      `Checkout BaseHarbor candidate` (the other failures are cleanup post-conditions).
   4. `gh api .../jobs/120095197172/logs` → captured the failing `git fetch --depth=1 origin
      +refs/heads/2adae47a*` command, 3 retries, git exit 1, and job Input `ref: 2adae47a`.
   5. Root-caused: bare 40-hex SHA as checkout `ref` → refspec matches no branch/tag → git exit 1.
   6. `gh api .../commits/2f005813…` → C2 = single #1038 squash commit, parent 281d1ae0, 2 Go files only.
   7. Fetched `targeted-demo-acceptance.yml` at both `2adae47a` and `2f005813`; `diff` clean → workflow
      (incl. checkout/ref logic) unchanged by C2.
   8. Noted C2 not reachable from develop (develop HEAD past 7fbc1ec5) → the bare-SHA dispatch is off
      the develop lineage.
   9. Wrote verdict STILL-BROKEN(recipe) + owner recipe + read-only method note.
4. **Open / hand-off (no action this lane):** the failing step is a workflow-input defect, out of scope
   for the #1038/#1040 product fix; recipe documented for the workflow owner. No ledger/plan edits.
5. **Stop:** after file written + `O33-ATOMIC-DONE`.

O33-ATOMIC-DONE
