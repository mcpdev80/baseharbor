ROLLE: REVIEW
# PR #1050 independent review [done:cx1050]

## PLAN
- [x] 0. Read binding policies and dispatched prompt; establish review-only scope and record PLAN before review.
- [x] 1. Read repository/Planning rules; verify current PR/base/ownership and scope without modifying candidate.
- [x] 2. Inspect exact SHA diff, check availability of v0.4.25 example and validate supported schema and all four pins against integration candidates and release requirements.
- [x] 3. Run bounded local resolver probe for v0.4.26 and inspect existing CI evidence.
- [x] 4. Recheck candidate immutability; publish verdict with actionable solution recipe and stop criterion.

## Scope and tool limits
Assigned head: e551bc8bf2be88d2720deaca1317160256d49760; assigned base develop prefix: 1861ed07.
Only review and evidence are authorized. No implementation, merge, deployment, controller start or worker management.
No native TodoWrite tool is exposed in this session; this evidence checklist is the available persistent plan. Managed workspace-write sandbox remains in effect despite prompt requesting danger-full-access.
Turn-start basics flags: global events/reminders, unknown cx1050 model identity, idle f33-pinsdevelop, pool shortages, 16 open PLAN steps. These require inspection and scope triage; review lane must not assume coordinator ownership or restart paused production.

## Verdict
**DATA REVIEW PASS; MERGE BLOCKED by required CI.** No defect found in the one-file pins payload. This is independent source review, not Maintainer acceptance or release qualification. Literal comparison to the requested v0.4.25 pins example is unavailable: that file is absent from the reviewed tree; actual v0.4.23 precedent and the exact candidate's resolver/loader provide the schema authority instead.

## Identity and immutable source
- Reviewer: Codex, lane cx1050, ROLLE REVIEW; model family GPT-6 per runtime, exact backend/session UUID and Horde task ID not exposed. No Horde task or model identity is invented.
- Review date: 2026-10-23, live checks at 21:50–21:52 UTC; final SHA recheck at 21:52:12 UTC; host reports NTPSynchronized=yes.
- GitHub authenticated identity: mcpdev80 (only account used).
- PR: https://github.com/mcpdev80/baseharbor/pull/1050, OPEN.
- HEAD before and after inspection: `e551bc8bf2be88d2720deaca1317160256d49760`.
- Live develop/base before and after: `1861ed07888a1ec22c218ca491d23ee54f48e261`.
- Candidate tree: `a62209d0729aea4f5b3b3b46634bcf6b3373c767`.
- Reviewed three-dot and direct diff: only ADD `docs/releases/v0.4.26.consumer-pins.json`, mode 100644, 72 lines. `git diff --check` PASS.
- Pins file SHA256: `ba77fc3642913a68483485dca8067fed1fc3811c094d223ab4c6b26810788452`.
- Product and Planning remotes refreshed over HTTPS after configured SSH fetch failed on `/etc/ssh/ssh_config.d/20-systemd-ssh-proxy.conf` permissions. No SSH/system configuration changed. Planning main verified at `e3ee433623eda63139da4a7e8b31e95b9d6bc73b`.
- Related `core-923` remains active, owned by Flash-Coordinator (74e10b98); core-921/core-922/core-900 dependencies accepted according to `plan.py show core-923`. No claim taken, task metadata edited, or acceptance inferred.

## Acceptance evidence
| Requested check | Actual result |
| --- | --- |
| Exact SHA diff / one allowed file | PASS, as recorded above. |
| Schema against v0.4.25 example | Example absent. Existing v0.4.23 file has the same schema and three gates. Additive joint origins and ecosystem DAG match the current v0.4.26 resolver and are accepted by the current public loader. |
| Four immutable pins / source chain | PASS; connector and console integration candidates retrieved by full commit through GitHub API; Demo `baseharbor-core.ref` at full Demo SHA equals pinned Core. |
| release-requirements.json | PASS; all three pins gates required for v0.4.26; consumer role sets match dependencies after Core/Demo dependencies, and origin validation passes. Requirements define gate dependencies, not literal source SHA values. |
| Local resolver --tag v0.4.26 | PASS, exit 0; regenerated JSON byte-for-byte identical to submitted file. |
| Existing mandatory CI | FAIL; authenticated GitHub Actions app 15368, exact HEAD, run 40028406804 attempt 1, final check 120670776669. |

| Role | Verified full commit |
| --- | --- |
| Core | `1861ed07888a1ec22c218ca491d23ee54f48e261` |
| Demo | `25c319697fd3d89d894943e0f44cd3d5dde8b198` |
| Console | `69d1d14007fa43dc0c0457c3347280be7f802818` |
| Connector | `df1f4378d8c7fdc3e174a1e9dc8ad0155a955571` |

The Core pin intentionally identifies the source consumed by Demo/Console/Connector, not the later metadata-only PR HEAD; substituting PR HEAD would break the source-owned chain. The ecosystem graph is acyclic: Demo consumes Core; Console consumes Core/Demo; Connector consumes Core/Demo/Console.

## Reproduction
Review snapshots and assertion script: `evidence/board-2026-10-23/scripts/cx1050/`. The resolver and private loader were copied verbatim using `git show HEAD:scripts/...` with the full assigned candidate SHA, never from the dirty working tree.

```bash
python3 evidence/board-2026-10-23/scripts/cx1050/ecosystem_release_pins.py --tag v0.4.26 --core 1861ed07888a1ec22c218ca491d23ee54f48e261 --connector df1f4378d8c7fdc3e174a1e9dc8ad0155a955571 --demo 25c319697fd3d89d894943e0f44cd3d5dde8b198 --output evidence/board-2026-10-23/scripts/cx1050/resolved.json
python3 evidence/board-2026-10-23/scripts/cx1050/check_snapshot.py
```

Both PASS. The assertion script verifies byte equality, schema/gate precedent, all three requirements mappings, public loader, source origins and DAG. Initial loader invocation on the raw snapshot failed its file protection check because the shell umask produced mode 0664; repeating on the resolver's deliberately emitted 0644 output passed. This was a review snapshot permission issue, not a candidate defect. No expensive runtime or full release matrix was launched.

## Blocking finding and actionable solution recipe
**B1: trusted planner does not recognize the new pins path.** Existing run https://github.com/mcpdev80/baseharbor/actions/runs/40028406804/job/120670776669 fails with:

```text
BLOCKED: coverage-unresolved: unknown impact for docs/releases/v0.4.26.consumer-pins.json
BLOCKED: no required gates or validations
```

Trusted Plan, Workflow policy and Focused Python passed; final check failed; consumer/native jobs were skipped. Skips prove no consumer runtime success.

Worker recipe (Codex REVIEW does not implement):
1. Preserve the pins payload; create a separately assigned, reviewed CI-readiness prerequisite. In `scripts/pr-gate-plan.py` (unknown-path fallback at lines 1153–1155), add a narrowly bounded classification for the authorized release pins metadata path, selecting real validations of the public pins schema, graph and exact immutable chain. Do not classify all JSON or release files as safe and do not merely suppress unknown impact.
2. Add regression coverage in `scripts/test_pr_gate_plan.py`: the exact pins-only addition selects nonempty validations and resolves coverage; malformed/mismatched pins must fail the selected validator; unrelated JSON, unexpected files and mixed changes remain fail-closed. Reuse `scripts/test_ecosystem_release_pins.py`, `scripts/test_public_consumer_evidence.py` and existing executor interfaces. If a new validation command is necessary, wire it through the trusted planner/executor/final contract and test that it actually executes. Do not infer content validation from a filename-only planner test.
3. Qualify and land that CI prerequisite through its legitimate trusted-source path before using it to qualify #1050; a planner change in an untrusted candidate cannot qualify itself. Then normally integrate current develop into the existing PR, obtain review for the resulting exact SHA, and run affected required checks only. Do not rerun the unchanged known-red candidate without changed trusted inputs.
4. Test requirement: the local resolver byte comparison and public loader/DAG/origin checks must still pass; the selected CI validations must execute and pass; `PR Selected Checks` must succeed for the final SHA from app 15368.
5. Stop criterion: unknown impact, empty selected coverage, skipped/failed required validation, changed consumer chain, changed candidate without fresh review, or any red required CI means no merge/acceptance. No protection changes or CI bypass.

## repo-rule compliance and operational triage
Read-only candidate inspection throughout. Existing uncommitted product wizard edits, site/ and worktrees/ belong to other lanes and were preserved; their changes during this review were not used as source. Only this review evidence and isolated review snapshots/assertion helper were written. No product commit, PR comment, merge, release, deployment or worker dispatch occurred.

Global basics flags were inspected as coordinator observations, not assignments: discovered running lanes and coordinator-idle event do not prove completion; idle f33 does not authorize removal or re-tasking; pool shortages do not authorize dispatch in this review lane. Own PLAN is now complete; exact backend identity remains unavailable. Global events/reminders and unrelated PLAN steps remain for their existing coordinator; they were not falsely acknowledged as resolved. No controller was started.

Policy conflict recorded: local COST_CONTROL/GITHUB_AUTHORITY retain pause language, while Planning entrypoints describe a later controlled resumption/delegation. This bounded independent review is permitted under both and resolves no wider activation/merge authority question.

END CX1050-REVIEW-DONE — DATA REVIEW PASS; MERGE BLOCKED (required CI B1).

## Folgebefund
ROLLE: REVIEW — cx1050b

### PLAN
- [x] 0. Read binding policies and cx1050b prompt; record follow-up scope and PLAN before analysis. Native TodoWrite remains unavailable in the exposed tools.
- [x] 1. Refresh live PR and trusted source; trace Consumer Pins and Selected Runtime conditions with exact file:line references.
- [x] 2. Trace final meta-job enforcement and evaluate minimal content/label/input routes without triggering unnecessary full CI.
- [x] 3. Record supported minimal recipe, test expectations and stop criterion; recheck HEAD and finish review only.

Scope: append-only review evidence; no implementation, workflow dispatch, label changes, merge or worker management.

### Exact trigger finding
**There is no Consumer Pins `paths` filter in ci.yml.** The skip follows trusted planner outputs, not a missing JSON glob in the workflow. References below use trusted base `1861ed07888a1ec22c218ca491d23ee54f48e261`; workflow/planner content is unchanged in PR HEAD `e551bc8bf2be88d2720deaca1317160256d49760`.

| File:line | Actual rule / consequence |
| --- | --- |
| `.github/workflows/ci.yml:5–15` | `workflow_dispatch` has no inputs. PR events target develop/main with no top-level paths filter. |
| `.github/workflows/ci.yml:433–458` | develop PR planner checks out **base.sha**, computes exact diff, then runs the trusted base planner. Editing the planner only in #1050 cannot change its own trusted classification. |
| `scripts/pr-gate-plan.py:1074–1079` | Documentation rule admits docs paths ending `.md`/`.rst`, named top-level documentation/config files, or site/. The added `.json` does not match. |
| `scripts/pr-gate-plan.py:1153–1155` | JSON falls through to `unknown impact for docs/releases/v0.4.26.consumer-pins.json`. |
| `.github/workflows/ci.yml:464–469` | `consumer_mode` is derived from selected gates by the pinned dependency helper, not from changed consumer-pins filenames. |
| `scripts/pr-gate-dependencies.py:8–26` at pinned commit `aa8caaa571a1b3a374ae37af43b224d234c7b6a0` | Union catalog dependencies of selected gates; mode ecosystem only when that union includes console or connector. Empty selected gates give dependencies=[], mode=core. |
| `.github/workflows/ci.yml:509` | Consumer Pins requires PR event, develop base and `consumer_mode == 'ecosystem'`. Current core mode means skipped. |
| `.github/workflows/ci.yml:470–475` | `runtime_required` is computed by pinned selected-runtime helper. |
| `scripts/pr-selected-runtime-matrix.py:15–24` at pinned commit `45500525aa65957a22e6076ac28fe5cb6c6f9610` | Only selected `atomic/docker/*` or `atomic/podman/*` gates produce executable runtime rows. Empty selected gates give required=false. |
| `.github/workflows/ci.yml:764–765` | Selected Runtime additionally requires runtime_required=true, successful Exact Candidate/Runner Interface, and successful Demo Pin OR Consumer Pins. Current false output makes it skipped. |

Reproduction uses the actual downloaded `trusted-pr-plan` artifact from run 40028406804, retained as `evidence/board-2026-10-23/scripts/cx1050b/pr-plan.json`. It has coverage_status=unresolved, selected_gates=[], required_validation_checks=[], required_consumer_dependencies=[], and broad_required=true. Running the **exact pinned** dependency/runtime helpers read-only against this artifact produces:

```json
{"dependencies": [], "mode": "core"}
{"include": [{"gate": "not-required", "id": "not-required", "runtime": "static"}], "required": false, "unsupported": []}
```

`broad_required=true` records unresolved coverage here; it does not authorize or schedule the full release catalog.

### What the meta-job actually requires
- `.github/workflows/ci.yml:780–783`: final job waits for the named prerequisites and runs with always(). A job appearing in `needs` does **not** mean it must always have succeeded.
- `.github/workflows/ci.yml:816–822`: trusted-plan success, nonempty artifact, exact base/HEAD/integration identity, authenticated result collection. Final verifier is loaded from **pinned c6db56c4ac077dbc496134cb0871c8e996a593fe**, not arbitrary PR contents.
- `.github/workflows/ci.yml:823–834`: verify actual atomic/public-contract evidence when selected/executed, then execute final verifier.
- `scripts/pr-required-final.py:24–32` at that pinned commit: explicit resolved coverage, no unresolved reasons, and **at least one selected gate or validation**. These are the two failing requirements for #1050.
- Same verifier `:47–60`: authenticated Focused Python and Workflow policy success always; Exact Candidate/Runner Interface and consumer prerequisite depend on selected runtime gate families/dependencies; Public Contracts is required only when selected.
- Same verifier `:74–78,91–104`: exact authenticated prerequisite set and successful result for every selected gate/validation; a skipped selected requirement fails. Unselected runtime jobs may legitimately remain skipped.
- `.github/workflows/ci.yml:683`: validation placeholder explicitly prints `NOT REQUIRED (not PASS)`; its green job cannot fill empty required_validation_checks.

Thus Consumer Pins and Selected Runtime are **not independently mandatory for this metadata-only PR**. Making those two jobs run would neither resolve the unknown JSON impact nor establish sufficient coverage. The mandatory final check must pass with real, applicable validations, while unrelated runtime jobs may remain correctly unselected.

### Minimal supported recipe and rejected shortcuts
**No currently supported PR-content-only, label, or dispatch-input change can preserve the functional JSON file and make the existing trusted policy qualify it.** The prompt's suggested direct routes do not exist in this candidate's CI. A bounded trusted-policy prerequisite is necessary; do not invent a label or claim a rerun fixes it.

Rejected routes:
- Adding a Markdown companion selects DOC_CHECKS for that file but leaves the JSON's unknown-impact entry unresolved.
- Renaming the JSON to Markdown changes the required runtime-consumed path/format and fails the task. An empty commit or touching an unrelated known file leaves the original unresolved path.
- No PR label is read to select these jobs; ci.yml has no dispatch inputs. workflow_dispatch also fails the `event_name == 'pull_request'` guards and cannot create the required PR meta-job success by itself.
- Forcing integration gates selects work that this selected-runtime helper cannot execute (its supported rows are atomic Docker/Podman); forcing an atomic gate still does not fix the unknown JSON path.
- Consumer Pins `ci.yml:526` invokes resolver with Core=`github.sha` (synthetic PR integration), **not** the historical Core SHA in this metadata snapshot. Current immutable consumers reference Core `1861ed07...`; the existing resolver rejects a different Core at `scripts/ecosystem_release_pins.py:21–27`. Broadening its condition alone is therefore not a repair and would not validate the submitted file anyway.

Worker recipe — smallest correct behavioral change:
1. Keep #1050's JSON payload and immutable source chain unchanged. Prepare a separate, assigned CI-readiness change on the trusted develop path; narrowly classify the exact authorized release pins metadata path as requiring **one lightweight pins-data validation profile**, with no selected runtime gates. Avoid a generic docs-JSON allowlist.
2. `scripts/pr-gate-plan.py:1074` vicinity: emit that nonempty required validation for the authorized pins path. `scripts/pr-check-runner.py:52–58`: register its validation ID. `.github/workflows/ci.yml:682–719`: provide its real execution branch (or reuse an existing validation only after extending that validator to perform the same pins-specific checks). Do not change Consumer Pins/Selected Runtime conditions or final fail-closed checks.
3. The lightweight validator must read the **actual candidate JSON** and use its embedded Core/Connector/Demo SHAs, regenerate the v0.4.26 snapshot from immutable consumer sources, and compare canonical JSON plus validate public schema, DAG, origins and required gate roles. Use explicit `--core 1861ed07... --connector df1f4378... --demo 25c31969...`, derived from the file rather than hardcoded forever, and `--tag v0.4.26`; output only to runner-temp. Do not use synthetic integration SHA as the historical consumed Core. Existing cx1050 local probe proves this bounded check passes for the submitted data. Private-source access, if required by repository visibility, must use the existing authorized credential route; missing access blocks rather than silently skipping.
4. Add focused planner/executor/validator regressions: exact one-file addition resolves coverage with nonempty validation; actual current file passes; malformed/changed/missing/mismatched SHA or graph fails; unrelated JSON and mixed source changes retain their own coverage requirements. Test that the actual validator executes, not merely that its filename appears in a matrix.
5. Qualify this small CI prerequisite through the existing trusted-source review process, then integrate current develop normally into the original #1050 branch and obtain exact-SHA review/checks. Required result: resolved coverage, pins validator SUCCESS, Focused Python SUCCESS, Workflow policy SUCCESS, and `PR Selected Checks` SUCCESS from app 15368. `selected_gates=[]`, consumer_mode=core and runtime_required=false are correct for this metadata-only change; Consumer Pins/Selected Runtime may remain skipped as explicitly not required. No full runtime/release CI is needed.

This is the minimum semantic repair, not a claim of an implemented patch or a guaranteed smallest line count. The existing workflow has no pins-data validation ID that can simply be requested with a label/input; a new ID must be connected across planner, whitelist and executor together.

**Stop criterion:** do not merge while coverage is unresolved, required validations are empty/skipped/red, source access is uncertain, or required meta-check is red. Never relabel skipped required work as success or bypass CI. If the Owner requires both named consumer/runtime jobs to run regardless, that changes scope to integration qualification and requires its own exact consumer chain and executable gate coverage; it is not the minimal metadata repair.

### Follow-up verification and compliance
Live PR rechecked unchanged at HEAD e551bc8bf2be88d2720deaca1317160256d49760, base 1861ed07888a1ec22c218ca491d23ee54f48e261, OPEN, no labels; mcpdev80 only. Product and Planning HTTPS refresh succeeded; SSH permission failure reported by basics remains a coordinator configuration observation, not a stale-state assumption. Global pool/idle/reminder flags are outside this review assignment and were not used to dispatch or remove workers. No unprocessed global observations were acknowledged as resolved.

No product changes, labels, dispatches, CI reruns, merges or Planning state transitions performed. Native TodoWrite is not exposed; the evidence PLAN is complete. Policy-permitted independent review only.

END CX1050-FOLLOWUP-DONE — Rezept: trusted narrow pins-data validation prerequisite; retain JSON, validate its embedded immutable chain, select no runtime gates, require real validation plus green authenticated PR Selected Checks. No supported label/input shortcut.
