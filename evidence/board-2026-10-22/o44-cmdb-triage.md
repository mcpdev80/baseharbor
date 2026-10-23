# CMDBTRIAGE — run 39865161687 (Pre-Release · v0.4.26) triage

**Task:** delegation-inbox.txt line CMDBTRIAGE (READ-ONLY triage; WRITE-FIRST this file + PLAN)
**Run:** 39865161687 — workflow "Pre-Release · v0.4.26" · branch `develop` · head `2adae47afddc299d06dbc34eb16a8e011c14b6dc`
**Repo:** `mcpdev80/baseharbor` (run 39865161687 does NOT exist under owner `baseharbor`; correct owner is `mcpdev80`)
**Completion state:** completed (run-level failure; 5 jobs failed, rest skipped by upstream dependency)

## Matrix / inputs of the run
- `EVENT_NAME=workflow_dispatch`, `INPUT_TAG=v0.4.26`, `INPUT_DEMO_REF=896dc3cb` (short, non-immutable demo ref), `INPUT_CONSUMER_REF=` (empty)
- Candidate commit `2adae47` follows current `develop` release line (`git fetch origin main develop`; `fatal: couldn't find remote ref refs/heads/release/v0.4.26`).
- For `v0.4.26` the Source job runs `python3 scripts/ecosystem_release_pins.py --tag v0.4.26 --core <HEAD> --connector $INPUT_CONSUMER_REF --demo $INPUT_DEMO_REF --output docs/releases/v0.4.26.consumer-pins.json`.

## Failed jobs (5) — first error lines + classification

| # | Job id | Job name | conclusion | First 2 error lines | Class |
|---|--------|----------|------------|---------------------|-------|
| 1 | 120125410400 | Source · Contracts · Docs | failure | `raise ValueError('immutable source commit required')` / `ValueError: immutable source commit required` | PREVIEW-DEFECT |
| 2 | 120125508296 | v0421 ha groups · All runtime results | failure | `##[error]Process completed with exit code 1.` / summary block `SOURCE_RESULT: failure` | PREVIEW-DEFECT (cascade) |
| 3 | 120125508731 | runtime heavy gates · All runtime results | failure | `##[error]Process completed with exit code 1.` / summary `SOURCE_RESULT: failure` | PREVIEW-DEFECT (cascade) |
| 4 | 120125509121 | runtime light gates · All runtime results | failure | `##[error]Process completed with exit code 1.` / summary `SOURCE_RESULT: failure` | PREVIEW-DEFECT (cascade) |
| 5 | 120125552173 | Evidence · Candidate Manifest | failure | `##[error]Process completed with exit code 1.` / (artifact upload, upstream candidate missing) | PREVIEW-DEFECT (cascade) |

## Root cause (job 120125410400, verbatim stacktrace)
```
File ".../scripts/ecosystem_release_pins.py", line 136, in <module>  main()
File ".../scripts/ecosystem_release_pins.py", line 125, in main
  console_candidate = json.loads(source_file(REPOSITORIES['console'], candidate['console'], 'integration-candidate.json'))
File ".../scripts/ecosystem_release_pins.py", line 100, in source_file
  raise ValueError('immutable source commit required')
ValueError: immutable source commit required
```
The `console` entry inside `baseharbor-console`'s `integration-candidate.json` is a **short SHA** (`896dc3cb`), not a 40-char immutable commit. `source_file()` guard (`scripts/ecosystem_release_pins.py:96-100`) requires `exact(commit)` (40 hex chars) for all repositories → the console candidate fails. Jobs 2–5 are downstream skips-turned-failures triggered by `SOURCE_RESULT: failure`.

## Classification
- **ENV-OUTAGE:** none. Runner image healthy (ubuntu-24.04, runner 2.338.0), network fetches of `main`/`develop` succeeded. Single `fatal: couldn't find remote ref refs/heads/release/v0.4.26` is the expected "no release branch yet" path, not an outage.
- **PREVIEW-DEFECT (fixed-by #1038 in C2):** the immutable-commit guard is the real blocker. **#1038 does NOT fix this.** #1038 (`2f005813`, "plan authoritative zero-impact envelope for Edit preview") changes only `cmd/baha/app_edit_apply_result.go` + a Go test (`cmd/baha/app_edit_zero_preview_test.go`, 54 insertions); `git show --stat` of #1038 touches zero `.py`/`.json` files, and `git show 2f005813 | grep -c ecosystem_release_pins|integration-candidate|immutable` = 0. The pins-guard fix belongs to the v0.4.26 consumer-resolution line (#1020 `2b8095b9` touched `scripts/ecosystem_release_pins.py` + added `scripts/test_ecosystem_release_pins.py`; 16 console/pins hits), not to the Edit-preview envelope of #1038.
- **OTHER:** none beyond the cascade (jobs 2–5) which are PREVIEW-DEFECT consequences, not independent.

## Rows that flip green after C2-final
Run has 26 matrix rows. Failed=5, skipped=21 (all skipped depend on the Source job). C2-final HEAD is `9f4916be` (`docs(v0.4.26): correct release notes + changelog dates and attribution (#1041)`), which touches **only `CHANGELOG.md`** — zero edits to `scripts/ecosystem_release_pins.py`, `integration-candidate.json`, or the console integration-candidate ref. `git show 9f4916be | grep -c integration-candidate|console|immutable|pins` = 0.

**Rows that flip green after C2-final: 0.** The immutable-console-commit defect is untouched by C2-final; the run will continue failing at the Source job until the console `integration-candidate.json` `console` ref is set to a full 40-char immutable SHA (or the pins guard is relaxed for reviewed refs), which is neither #1038 nor C2-final.
