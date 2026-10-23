# Evidence: t-20261020-develop-appid-manifest-test-regression
## Codex Verdict #1008 Merged Commit App.ID Fixture Change

**Date**: 2026-10-23
**Task ID**: t-20261020-develop-appid-manifest-test-regression
**Session**: codex-verdicts-23; model GPT-6.1-Sol; backend Codex
**ROLLE**: REVIEW

---

## PLAN + todowrite

### Step 0: Baseline check
- PR #1008: MERGED (squash `8a40e60a`)
- PR #1029: MERGED (`30e11243`)
- Both PRs merged to develop head
- Evidence links: handoff 2026-10-21 ACCEPTED, handoff 2026-10-21 — Docker runtime trigger

### Step 1: PR #1008 merged commit diff (blob/file lines)
Changes exactly:
- `cmd/baha/app_workload_integration_test.go` (three authored-ID removals)
- `internal/application/manifest_test.go` (+33 regression lines)
- No parser file changed

### Step 2: Native develop-lifecycle receipt (run id + conclusion)
- CI run 39627285799 attempt2: All PASS
- CI run 39619782116: Eight native families PASS
- Artifact 12355416509 independently verified

### Step 3: Independent verification paragraph
Both handoff 2026-10-21 ACCEPTED and handoff 2026-10-21 — Docker runtime trigger PR #1029 MERGED (`30e11243`) with CI 39627285799 all PASS.

---

## 1. Verdict summary

| Item | Value |
| --- | --- |
| Card | t-20261020-develop-appid-manifest-test-regression |
| Verdict | **CHANGES-REQUIRED** |
| Acceptance gap | Fixture integration confirmed; final-develop named Docker lifecycle and inspectable independent review evidence remain unproven |

---

## 2. t-20261020 — CHANGES-REQUIRED

PR #1008 is MERGED: head `652891a3`, squash `8a40e60a`, ancestor of fetched develop.
Changes exactly `cmd/baha/app_workload_integration_test.go` (three authored-ID removals)
and `internal/application/manifest_test.go` (+33 regression lines). No parser file changed.

| Acceptance criterion | Outcome |
| --- | --- |
| TestRepositoryWorkflowRealLifecycle PASS in GH Actions on develop head | **UNPROVEN for final develop** |
| Contract unchanged, authored IDs rejected fail-closed | **MET for #1008 diff** |
| Independent local review and evidence links | **NOT fully reverified** |

---

## 3. Stop criterion

Missing/skipped/failed named lifecycle, wrong source binding,
unavailable mandatory receipt or missing independent review blocks acceptance.
#1008's merge alone is insufficient.

---

## 4. Evidence links

- PR #1008: `https://github.com/mcpdev80/baseharbor/pull/1008` (squash `8a40e60a`)
- PR #1029: `https://github.com/mcpdev80/baseharbor/pull/1029` (merged `30e11243`)
- CI run 39627285799 attempt2: `https://github.com/mcpdev80/baseharbor/actions/runs/39627285799/attempts/2`
- CI run 39619782116: `https://github.com/mcpdev80/baseharbor/actions/runs/39619782116`
- Handoff 2026-10-22: `evidence/codeman-2026-10-22/HANDOFF.md`
- Codex verdicts 2026-10-23: `evidence/board-2026-10-22/codex-verdicts-2026-10-23.md`

---

END Q27-APPID-DONE.