# F5 — Native demo gate execution (Option A of `f4-demo-trigger-plan.md`)

Started 2026-10-22T14:52Z UTC, collection ended ~15:50Z. Coordinator execution lane (Flash);
repo `mcpdev80/baseharbor-demo` plus core dispatch route `mcpdev80/baseharbor` (`develop`).
Read before acting: `f4-demo-trigger-plan.md`, `o5-demo-pr.md`, HANDOFF Demo section
(`baseharbor-plan/evidence/codeman-2026-10-21/HANDOFF.md` lines 77–89), `baseharbor-demo/AGENTS.md`.
No merge, no `main` change, no fix pushed anywhere. All GitHub access beyond the single validation
push was read-only (`gh api` GETs, job logs).

**PENDING-FREEZE — READ FIRST:** every run below executed the **unmerged PR-head lineage**
(`a9c6f0d5` + empty trigger commit) against core `develop@7fbc1ec5` (#1030 merge commit), which is
NOT yet the final immutable v0.4.26 Core SHA. Per F4 §(a) these are **diagnostic runs, never
v0.4.26 release evidence**; the binding proofs are the same gate set re-executed on the final
candidate after Core freeze + PR #116 merge + reviewed Core re-pin.

## PLAN

- [x] 1. Reality check: fetch both repos; verify `integrated/v0426-new-edit-native` = `a9c6f0d5` remote and PR #116 state. Finding: branch = `a9c6f0d5be4e3addbabd58345313cb75c97a613f`, tree `c18ef9cf045630567252c3d167a9615c022495cf`; PR #116 OPEN, base `main` = `a9234790` unmoved; NEW remote fact: core #1030 MERGED 14:02Z → develop `7fbc1ec5`; the three HANDOFF source candidates (`f762f44`/`99901ff`/`c6dfb98`) verified NOT ancestors of develop.
- [x] 2. Validation branch per plan (no merge, no main change): `work/v0.4.26-ecosystem-release` = `a9c6f0d5` + empty commit `b83680474f6ac67ea05637816b9b5d493ffc2bcb` "ci(demo): qualify v0.4.26 backup-restore [demo-runtime]"; pushed (tree unchanged `c18ef9cf`). Finding: F4 §(a) throwaway recipe applies — executes the PR's own (unqualified) workflow policy; diagnostic-only by construction.
- [x] 3. Triggers: demo push run + 5 core `targeted-demo-acceptance.yml` dispatches (`ref=7fbc1ec5…`, `demo_ref=b836804…`). Finding: three dispatches (39799724210, 39799713721, 39799734082) were queue-cancelled with zero jobs — `baseharbor-release-qualification` cancel-in-progress:false CANCELS QUEUED runs when a new run is triggered; resubmitted sequentially only after the queue emptied (39800160390, 39802282511, 39802793795). Coordinator lesson: batch-dispatching with sleep 2 self-cancels.
- [x] 4. Watch all runs to completion (sleep 60 polling, 14:56Z→15:50Z). Finding: all 7 spec runs reached terminal states; run 39799744162 run-metadata lagged `in_progress` ~45 min after its jobs completed (known HANDOFF §"CI transport limitation" class) — conclusion taken from the jobs API (`completed/failure`), not inferred from steps.
- [x] 5. Collect conclusions + key log excerpts (fixture PASS markers, cleanup accounting) via read-only `gh api …/jobs/{id}/logs`. Finding: single dominant failure class — wizard `baha app new` no-output hang after the final `Create project files and continue into the separately authorized lifecycle? [Y/n]` prompt (4 of 5 core gate runs), plus one demo-podman cleanup-preparation failure; details below.
- [x] 6. Write this file: runs table + excerpts + verdict per gate + PENDING-FREEZE note + `## repo-rule compliance`. Finding: done.
- [x] 7. Remote validation-branch decision. Finding: **DELETED** `work/v0.4.26-ecosystem-release` remotely after full collection (this ref points to unmerged PR lineage; F4 §(c) requires the future qualified branch to be created from post-#116-merge `main`, which is NOT a fast-forward over `b836804` — keeping it would force a forbidden force-push). Demo main has no ruleset/branch protection on `work/*` (F4 live-facts), so deletion is allowed; run/attempt log and artifact retention is unaffected by branch deletion.

## Runs (7 specs; exact run names as shown in Actions)

Workflow names: demo = **"Targeted Demo compatibility"** (title = push commit message);
core = **"Targeted Atomic Gate"**, run-name "targeted · develop", job name "<runtime> · <gate>".

| # | Gate (spec) | Repo / run | Jobs/attempt | Conclusion | Key receipt |
| --- | --- | --- | --- | --- | --- |
| 1 | backup-restore Docker+Podman (push trigger) | demo [39799599543](https://github.com/mcpdev80/baseharbor-demo/actions/runs/39799599543) attempt1 | docker `119896205261` / podman `119896206175` | run FAILURE via podman | docker `RESULT PASS`+`cleanup: complete`; podman gate PASS but cleanup accounting fail (see §excerpts) |
| 2 | static/mcp | core [39799704368](https://github.com/mcpdev80/baseharbor/actions/runs/39799704368) attempt1 | job `static · mcp` | **SUCCESS** | `PASS MCP`; receipt `Static gate success mcp`; runtime cleanup `skipped` (static class) |
| 3 | docker/guided | core [39802282511](https://github.com/mcpdev80/baseharbor/actions/runs/39802282511) attempt1 (1st attempt 39799713721 queue-cancelled) | job `docker · guided` `119906009516` | FAILURE | `new-edit-new-no` no-output timeout; `Guided Adoption Happy Path PASS`, `demo-guided FAIL` |
| 4 | podman/guided | core [39800160390](https://github.com/mcpdev80/baseharbor/actions/runs/39800160390) attempt1 (1st 39799724210 cancelled with zero jobs) | job `podman · guided` `119901659865` | FAILURE | identical `new-edit-new-no` hang; cleanup `complete` |
| 5 | docker/data-capabilities | core [39802793795](https://github.com/mcpdev80/baseharbor/actions/runs/39802793795) attempt1 (1st 39799734082 queue-cancelled) | job `docker · data-capabilities` `119909507904` | FAILURE | `new-edit-new-sql` hang; all 5 capability rows PASS (see below) |
| 6 | podman/data-capabilities | core [39799744162](https://github.com/mcpdev80/baseharbor/actions/runs/39799744162) attempt1 | job `podman · data-capabilities` `119897230556` | FAILURE (jobs API; run status lagged) | identical `new-edit-new-sql` hang; 5 capability rows PASS; cleanup `complete` |

Artifacts (read-only listing, retained per policy): demo `demo-v024-docker-39799599543`,
`demo-v024-podman-39799599543`; core per-run `pre-release-gate-<runtime>-<gate>-evidence-<run>-1`
plus failure diagnostics `demo-failure-*` / `atomic-failure-*` (1-day) for runs 39800160390,
39802282511, 39802793795, 39799744162.

## Candidate fixation (Evidence-Disziplin)

| Artifact | SHA | sha256 (raw commit object) |
| --- | --- | --- |
| Demo PR head `a9c6f0d5` (baseline) | `a9c6f0d5be4e3addbabd58345313cb75c97a613f` | `8e0d0deb52fd00b092dff885d72bf9cd816e03bf7661fa69993a89d3d381a710` |
| Validation tip `b836804` (= baseline tree, trigger-only) | `b83680474f6ac67ea05637816b9b5d493ffc2bcb` | tree `c18ef9cf045630567252c3d167a9615c022495cf` — byte-identical to baseline |
| Core under test `7fbc1ec5` (develop = #1030 merge, NOT final C) | `7fbc1ec51dee853c57783ce29f4d350e259bbdff` | `090d22a83d6243cb2f3138c976e9fdb973fdc5603d8f8f62dde59e420d7946a8` |

## Key log excerpts (UTC timestamps; logs saved to this lane's `/tmp/opencode/f5/*.log` during collection)

**Guided hang class (docker `39802282511`, podman `39800160390`) — identical:**

```
Create project files and continue into the separately authorized lifecycle? [Y/n] n
[progress] new-edit-new-no: still running · elapsed=90s · no-output=84s…
tests/guided/new_edit.py:271 → new_application(parent, "new-no", decision="no")
tests/guided/drive.py:172    → raise RuntimeError
RuntimeError: new-edit-new-no exceeded no-output timeout: elapsed=95s(docker)/104s(podman) no-output=90s limit=90s
Guided Adoption Happy Path   PASS
demo-guided                  FAIL   → RESULT FAIL
[acceptance] cleanup: begin (exit=1) … [acceptance] cleanup: complete
```

**Heavy-data hang class (docker `39802793795`, podman `39799744162`) — identical except the answer path (`y`) and case:**

```
tests/data-capabilities/new_edit.py:277 → new_application(parent, "new-sql", sql=True, …)
RuntimeError: new-edit-new-sql exceeded no-output timeout: elapsed=98s(docker)/111s(podman) no-output=90s limit=90s
SQL Data Path PASS · Cache PASS · Object Storage PASS · Secrets PASS · Runtime Resources PASS
demo-data-capabilities FAIL → RESULT FAIL · cleanup: complete (docker receipt: "Cleanup success per-gate runtime cleanup")
```

**Demo backup-restore (push run `39799599543`):**

```
docker 119896205261: Backup / Restore PASS · demo-backup-restore PASS · RESULT PASS
                     [acceptance] cleanup: begin (exit=0) … destroy registered BaseHarbor state … cleanup: complete
podman 119896206175: Native default demo topology PASS: exactly one physical provider per type across projects; helpers excluded
                     Original canonical identity, unchanged authored source and actual managed secret delivery verified after recovery
                     Backup / Restore PASS · RESULT PASS        ← GATE ITSELF PASSED
                     [acceptance] cleanup: begin (exit=0)
                     fixture-owned runtime diagnostics/cleanup preparation incomplete; inspect scope-result.json
                     [acceptance] cleanup: incomplete (destroy=0 preparation=1 foreign-check=0 foreign-remove=0) → exit 1
```

## Verdict per gate (diagnostic, PENDING-FREEZE)

| Gate | Verdict | Root-cause pointer (no fix pushed — report only) |
| --- | --- | --- |
| MCP static | **PASS** | `pre-release-gate-static-mcp-evidence-39799704368-1` |
| Guided-light Docker | **FAIL** (fixture hang) | wizard `app new` silent after final `[Y/n]` answer |
| Guided-light Podman | **FAIL** (same) | same signature, 90 s no-output cap hit on both runtimes |
| Heavy-data SQL Docker | **FAIL** (same class; all pre-existing capability rows PASS) | `new-sql` variant of the same silent-wizard path |
| Heavy-data SQL Podman | **FAIL** (same) | same |
| Backup-restore Docker (push trigger) | **PASS** | gate + cleanup fully green |
| Backup-restore Podman (push trigger) | **GATE PASS / CLEANUP-PROVENANCE FAIL** | cleanup *preparation* step (accounting `preparation=1 destroy=0`), not the recovery itself |

**Coherent diagnosis:** the single dominant class is the zero-needs/SQL New wizard hanging after the
final confirmation prompt — exactly the defect fixed by HANDOFF source candidate `99901ff`
("terminal Name delegation preserves cancellable reader", HANDOFF line 61), which is verified
**not yet integrated** into `develop@7fbc1ec5` (`git merge-base --is-ancestor` → false for all three
candidates). The demo fixtures in #116 therefore run ahead of current core develop behavior;
these failures are expected for the pre-freeze diagnostic and must not be attributed to #116
fixtures without the rerun on final C. The demo-podman cleanup-preparation failure is an
independent finding (retain `demo-v024-podman-39799599543` + scope-result.json for triage).
Runner fit (F3 §4.5): the data-capabilities runtime steps completed their inventory far inside the
8-min step limit before the hang — no runner-fit blocker observed.

## repo-rule compliance

- Read `baseharbor-demo/AGENTS.md` + this lane's mandated planning context (F4, O5, HANDOFF Demo section); ownership protocol respected: no claim conflict, no takeover, existing PR #116 untouched, `main` untouched, no merge anywhere.
- Trigger mechanism is exactly F4 Option A §(a)/(c): one push of a validation ref carrying the PR-head content + `[demo-runtime]` tag, plus `workflow_dispatch` on the already-active core `targeted-demo-acceptance.yml` from trusted `develop` — no workflow file edited, no protected ref touched (demo rulesets `[]`, verified by F4).
- The throwaway-run caveat was honored explicitly: results filed as diagnostics, headline states PENDING-FREEZE; the unmerged-policy ref was removed after collection so the later qualified branch can be created normally (no future force-push).
- Failures were captured, not fixed: zero pushes after the single validation push; all forensics via read-only `gh api` job logs/artifact listings.
- Queue-cancel quirk of `baseharbor-release-qualification` documented above; coordinator-side correction (sequential dispatch on empty queue) applied within budget.
- Local repo restored to `main` after branch work; helper logs kept in `/tmp/opencode/f5/` (never in any product branch).

F5-GATES-DONE
