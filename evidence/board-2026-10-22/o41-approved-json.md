# o41-approved-json: canonical release-approved.json + coverage-provenance producer audit at develop HEAD

> READ-ONLY mission. Continues q25-provenance PARTIALLY result. No code executed, no runs dispatched. All facts from `gh api` against `mcpdev80/baseharbor` and local source inspection at develop HEAD.

## PLAN

1. Locate producer of canonical `release-approved.json` + coverage-provenance artifacts (scripts + workflow names, exact paths).
2. State precisely what must exist for v0.4.26 (inputs, run authorization, artifact names).
3. Query `gh api` for anything already emitted on develop 2adae47a..current.
4. Deliver exact gap list mapped to W1 vs qualify card.
5. END O41-APPROVED-DONE + compliance.

## FINDINGS

### 1. Producers located (exact paths, develop HEAD)

**Release-approved.json producer**
- Script: `baseharbor/scripts/pre-release-resume.py` — `main()` mode `approve`, line 656:
  `(args.output / 'release-approved.json').write_text(json.dumps(approval, indent=2) + '\n')`
  - Approval payload schema `baseharbor.pre-release.approval/v2`; for tag in {v0.4.24, v0.4.25, v0.4.26} it appends `consumer_ref` and **requires an immutable 40-hex consumer ref** (`pre-release-resume.py` ~line 690: `raise ValueError(f'{args.tag} requires an immutable complete consumer origin')`).
  - `release-approved.json` is written **only** in the `approve` mode, **after** `verify_coverage()` passes on a complete CMD-B run.
- Workflow: `baseharbor/.github/workflows/pre-release.yml` (develop HEAD sha `2995fdb16c…`, 90771 bytes). The `Evidence · Candidate Manifest` job calls `pre-release-resume.py approve` → emits the artifact.

**Coverage-provenance producer**
- Script: `baseharbor/scripts/pre-release-resume.py` mode `plan` writes `evidence-coverage.json` (the coverage-provenance artifact) in the same run, before approval.
- Coverage verifier + binding: `baseharbor/scripts/pr-gate-plan.py` (develop HEAD sha `6c38d65126…`, 1174 lines, v0.4.26). `trusted_binding_checks()` (~line 570) binds CI gate coverage to approved product paths from `baseharbor/docs/internal/ci-binding-qualifications.md`.
- Qualification board: `baseharbor-plan/tasks/t-20261017-release-0-4-26-qualify.md` (`#status/planned`, release v0.4.26, acceptance). Gate owner in f9-rerun-prep: concurrency group `baseharbor-release-qualification`, single W1 dispatch.

### 2. What must EXIST for v0.4.26 (canonical gate)

| Requirement | Source | Status for v0.4.26 |
|---|---|---|
| `tag=v0.4.26`, `--ref develop`, one full `workflow_dispatch` (`only_gates=""`, CMD-B = all 65 rows) | pre-release.yml | Not yet run on C2 |
| `ref={C2}` = current immutable develop head (`{C2}` = 2f005813 per f9-P0-1) | pre-release.yml `Resolve release candidate` | develop HEAD moved to 2f005813 |
| `demo_ref={D2}` = exact 40-hex baseharbor-demo SHA, matching `docs/releases/v0.4.26.demo-ref` | pre-release.yml:139-156 | **MISSING** — demo pin not frozen (f9-P0-2) |
| `consumer_ref` = immutable 40-hex connector SHA; required for v0.4.24/25/26 (`{N40|""}`) | pre-release-resume.py ~690 | **MISSING** — pins not resolved (f9-P0-3/P0-4) |
| `release/v0.4.26` branch == develop HEAD (immutable source commit) | pre-release.yml `Resolve release candidate` | **MISSING** — branch not found (404) |
| `docs/releases/v0.4.26.demo-ref` present, 40-hex | pre-release.yml:139 | **MISSING** |
| `docs/releases/v0.4.26.consumer-pins.json` U-pins {C2,D2,U40} | pre-release.yml:133-137 | **MISSING** |
| `release-approved.json` (approval/v2 + consumer_ref + coverage_digest) | pre-release-resume.py ~656 | **DOES NOT EXIST** anywhere (local + no run produced it) |
| `evidence-coverage.json` (coverage/v2 provenance) | pre-release-resume.py plan mode | **DOES NOT EXIST** (no full run landed) |

### 3. gh api results: anything already emitted on develop 2adae47a..current?

**Pre-release runs queried (all 5, all conclusion=failure):**

| run_number | id | head_sha | conclusion | updated_at |
|---|---|---|---|---|
| 215 | 39864801187 | 2adae47a… | failure | 2026-10-22T22:25:40Z |
| 216 | 39864813979 | 2adae47a… | failure | 2026-10-22T22:26:17Z |
| 217 | 39865161687 | 2adae47a… | failure | 2026-10-22T22:28:41Z |
| 24 | 39866644958 | 940fa694… | failure | 2026-10-22T22:40:26Z |
| 25 | 39898773138 | c31d654d… | failure | 2026-10-23T04:09:08Z |

**Root cause (verified in run 217 logs, job "Source · Contracts · Docs" step "Resolve release candidate"):**
```
fatal: couldn't find remote ref refs/heads/release/v0.4.26
ValueError: immutable source commit required
```
The `Resolve release candidate` job aborts (exit 1) before any gate runs because the `release/v0.4.26` branch does not exist. Every later job is `skipped`.

**Artifacts emitted by run 217 (the only run whose artifact is reachable):**
- Artifact `pre-release-final-evidence-39865161687-1` → contains **only `run-metrics.json`** (job outcomes), **no `release-approved.json`, no `evidence-coverage.json`**.
- Artifact `pre-release-source-evidence-39865161687-1` → source snapshot of the failed run.
- Confirmed by downloading the artifact zip (SHA256 `9c019719e4…`) and extracting: single file `run-metrics.json`, zero approval/coverage JSON.

**Local disk:** zero `release-approved.json`, zero `evidence-coverage.json` anywhere in the workspace.

**Conclusion of the gh query:** **Nothing** on develop 2adae47a..current has emitted the canonical `release-approved.json` or the coverage-provenance `evidence-coverage.json`. All 5 pre-release runs failed at source resolution; none reached the `approve` step.

### 4. Exact gap list → W1 vs qualify card

**W1 (dispatch vehicle; the ONLY job that can emit the canonical artifacts — NOT a fix target, only an executor):**
- W1 is a single `pre-release.yml` `workflow_dispatch --ref develop`, inputs `tag=v0.4.26, ref={C2}, demo_ref={D2}, consumer_ref={N40|""}, only_gates=""` (f9-rerun-prep §4/§5). W1 cannot be dispatched until the P0 preconditions below exist; W1 itself only produces the artifacts, it does not create the inputs.

**Gaps that must be closed BEFORE W1 (qualify card `t-20261017-release-0-4-26-qualify.md` responsibility, P0 preconditions):**
1. **Missing `release/v0.4.26` branch (== develop HEAD).** Fix: merge the qualify-ready source so a `release/v0.4.26` ref exists == `{C2}`. → pre-release.yml "Resolve release candidate" currently 404s. [qualify]
2. **`{C2}` not frozen.** develop HEAD moved from 2adae47a → 2f005813 (#1038 merged). Every receipt at 2adae47a is digest-void (o17 §3d). Rekey + rerun-all-65 required. → pre-release.yml `ref={C2}` input. [qualify/W1-source]
3. **`demo_ref={D2}` not pinned.** `docs/releases/v0.4.26.demo-ref` absent; `INPUT_DEMO_REF` on run 217 was truncated (`896dc3cb`), `INPUT_CONSUMER_REF` was `""` → the run's own guard `test "$INPUT_DEMO_REF" = "$pinned_demo_ref"` would also fail. → f9-P0-2. [qualify]
4. **`consumer_ref={N40}` not resolved.** v0.4.26 (like v0.4.24/25) demands an immutable 40-hex connector ref at approve; `integration-candidate.json` pins not all-40-hex; #7/#8 consumer merges outstanding. → pre-release-resume.py ~690 consumer_ref guard. [qualify]
5. **`docs/releases/v0.4.26.consumer-pins.json` U-pins missing** ({core {C2}, demo {D2}, console {U40}}). → pre-release.yml:133-137. [qualify]
6. **No run reached the `approve` step.** Until gaps 1-5 clear, no `release-approved.json` / `evidence-coverage.json` can be written. [qualify]

**Mapping:** all 6 gaps are **qualify-card inputs/preconditions (P0)**, not W1 rows. W1 (the `baseharbor-release-qualification` single dispatch) is the sole authorizing vehicle and will emit both artifacts *iff* gaps 1-5 are closed. No gap is a code defect to patch in a PR; each is a frozen-input / branch / merge precondition owned by the qualify stage.

## CONCLUSION

- **release-approved.json producer** located: `baseharbor/scripts/pre-release-resume.py` (approve mode, line 656) via `baseharbor/.github/workflows/pre-release.yml`.
- **Coverage-provenance producer** located: `baseharbor/scripts/pre-release-resume.py` (plan mode, `evidence-coverage.json`) + `baseharbor/scripts/pr-gate-plan.py` `trusted_binding_checks()` (line 570) / `docs/internal/ci-binding-qualifications.md`.
- **Must exist for v0.4.26**: full single CMD-B dispatch (`only_gates=""`), `ref={C2}` with `release/v0.4.26` branch == develop HEAD, exact 40-hex `demo_ref={D2}`, immutable `consumer_ref={N40}` (v0.4.26 requires it), present `v0.4.26.demo-ref` + U-pinned `consumer-pins.json`. Then `release-approved.json` (approval/v2 + consumer_ref + coverage_digest) and `evidence-coverage.json` (coverage/v2) are written by the approve job.
- **Already emitted on 2adae47a..current:** **NONE.** All 5 pre-release runs (215/216/217/24/25) failed at "Resolve release candidate" (`immutable source commit required`; `release/v0.4.26` branch missing / 404). Run 217's only artifact is `run-metrics.json` — no release-approved/coverage JSON. Local disk: zero.
- **Gap list (all qualify-card P0, not W1 rows):** (1) missing `release/v0.4.26` branch, (2) `{C2}` not frozen (develop moved 2adae47a→2f005813, all receipts digest-void), (3) `demo_ref={D2}` not pinned, (4) `consumer_ref={N40}` not resolved, (5) `consumer-pins.json` U-pins missing, (6) no run reached `approve`. W1 is the single authorizing vehicle that emits the artifacts once these six inputs exist.

**Q41-APPROVED-DONE:** read-only, no execution, no dispatch. Evidence: gh api run/artifact queries; run-217 artifact extracted (only `run-metrics.json`); local zero-find; source inspection at develop HEAD 2f005813.

## REPO-RULE COMPLIANCE
- Read-only: no product code changed, no workflow dispatched, no runs created, no files written into any product branch (this evidence file is in the local evidence board only).
- Evidence reproduced: run ids, head SHAs, artifact extraction, failure log quoted verbatim.
- No claim of any green gate; the five pre-release runs are reported honestly as failures.
- No secrets/credentials logged; artifact download used `gh api` (no token in evidence).
