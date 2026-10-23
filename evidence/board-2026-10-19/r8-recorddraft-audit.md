# R8: Read-only audit of draft-record-pr.md (record-PR prep by another lane)

Scope: verify the draft `evidence/board-2026-10-19/draft-record-pr.md` against
`e2060ea0:scripts/pr-gate-plan.py`, `e2060ea0:scripts/pr-gate-map-v1.json`, and
the diff-base/pr-gate classification. Read-only. I authored nothing in this repo.

## (1) JSON skeleton vs `_valid_binding_record` / `load_binding_qualification_record`

Loader (`e2060ea0:scripts/pr-gate-plan.py:441-492`):
- `load_binding_qualification_record(target_base_sha, repository='.')` requires
  `len(target_base_sha)==40`; parses JSON; calls `_valid_binding_record`.
- `_valid_binding_record` (`pr-gate-plan.py:475-492`) requires **exactly**:
  `repository`, `target_branch`, `reviewed_head`, `files`, `file_hashes`,
  `required_checks` (all non-None). Plus:
  - `reviewed_head`: str, len 40
  - `files`: non-empty dict
  - `file_hashes`: non-empty dict
  - `required_checks`: non-empty list

**DISCREPANCY A (blocker).** The draft skeleton (`draft-record-pr.md:21-51`)
does NOT match the loader. It omits every field the loader actually requires:
`files` and `file_hashes` are absent; `reviewed_head` is spelled
`pins.reviewed_head` (loader reads top-level `record['reviewed_head']`);
`required_checks` is absent. The draft also contains fields the loader never
reads (`coordinator_session`, `ruling`, `bootstrap`, `pins.repository`,
`pins.target_branch`, `pins.reviewed_head`, `pins.required_record_path`,
`pins.required_record_hash`, `pins.prerequisites[]`, `load_from_trusted_base.*`).

The loader at `pr-gate-plan.py:747-758` additionally requires:
  - `identifiers['head_sha'] == record['reviewed_head']` (40-hex head == the
    pinned reviewed head),
  - `_binding_inventory_matches(diff, record)` — every diff change path must be
    in `record['files']` with identical status, no extras (`pr-gate-plan.py:495-525`),
  - `_binding_content_hashes_verified(head_sha, record, repo)` — for each
    `path->expected` in `record['file_hashes']`, `git cat-file blob <head>:<path>`
    content sha256 must equal `expected` (`pr-gate-plan.py:528-555`).

So a correct record must contain `{repository, target_branch, reviewed_head,
files:{path:status}, file_hashes:{path:sha256}, required_checks:[ids]}` plus the
`files`/`file_hashes` inventory must equal the actual PR #1007 diff exactly. The
draft never encodes `files`/`file_hashes` as required; **the record as drafted
cannot pass `_valid_binding_record` and therefore fails CI.**

## (2) blob-hash commands: loader compares sha256 of *blob content*

`_binding_content_hashes_verified` (`pr-gate-plan.py:547-554`):
```
out = git('cat-file', 'blob', head_sha + ':' + path)   # raw blob bytes
digest = hashlib.sha256(out.stdout.encode('utf-8')).hexdigest()
```
`git cat-file blob <rev>:<path>` returns the **raw blob object** (no sha1
header prefix). So `git cat-file blob <rev>:<path> | sha256sum` equals the
loader's digest. **The command form is correct.**

**DISCREPANCY B (blocker).** The draft's "File 1" command
(`draft-record-pr.md:56-57`) references blob `09b480710cd23535b9a57c6c6f8a8305684a714f`:
```
git -C .../baseharbor cat-file blob 09b480710cd23535b9a57c6c6f8a8305684a714f | sha256sum
```
That blob is a *different, older revision* of `pr-gate-plan.py` (from commit
`ae1a7d31 [v0.4.26] CI: provide conservative area-to-gate mapping (#928)`), **not**
the e2060ea0 loader (`c199302e88cb1291d163d369fb85d576a0d5a0166c1b5ca6afbe47a7e718a8a8`).
The draft even asserts the wrong blob id in the verified list (`:68`:
"blob 09b480710..."). The loader in the record must be pinned to the exact
blob whose sha256 it documents; a stale-loader blob id is unverifiable against
the current gate. Same class of error for the `{{998_SHA}}` / map-hash
placeholders.

## (3) Bootstrap prose-bucket claim — WRONG bucket, no coverage-unresolved

The draft (`draft-record-pr.md:176-191`) asserts that a PR adding **only**
`scripts/ci-binding-qualifications.json` lands in the `prose` bucket
(`areas.prose.exact == []`) and would therefore need the Codex 3a bootstrap
exemption.

**DISCREPANCY C (blocker).** That is false. The map file `pr-gate-map-v1.json`
(`areas.prose.prefixes == ["atomic/static/","integration/static/"]`) is **never
consumed** by `plan()` — `grep` finds no `mapping['areas']`/`prefixes`/`exact`
use in `pr-gate-plan.py`. Classification is fully hardcoded in
`pr-gate-plan.py:967-1084`. The exact matching line for this PR is:
```
pr-gate-plan.py:1074-1079   elif (path.startswith('scripts/ci-') or path.startswith('scripts/pr-') or
                              path.startswith('scripts/test_pr_') or path.startswith('scripts/verify-') or 'evidence' in path):
                              area = 'ci-executor-unresolved'
                              require(CI_CHECKS)
                              unresolved.append('CI execution/evidence impact requires specific qualification: ' + path)
```
`scripts/ci-binding-qualifications.json` matches `scripts/ci-`, so it classifies
**`ci-executor-unresolved`**, not `prose`. And crucially, the bootstrap clause
(`pr-gate-plan.py:747-768`) resolves **only when `head_sha == H`** (the binding
PR is the reviewed candidate) with a clean integration merge — the "policy file
on develop but candidate head still unfrozen" scenario the draft describes
returns **unresolved** (fail-closed), per the docstring at `pr-gate-plan.py:744-746`.
So a PR adding only the policy file to develop does NOT resolve via the standard
pipeline; there is no `prose` exemption path, and `ci-executor-unresolved`
requires `CI_CHECKS`. The draft's "deadlock is a deliberate design bypassing
validated_diff" claim is unsupported.

## (4) required_checks == CI_CHECKS

Draft (`draft-record-pr.md:73-91`) documents `CI_CHECKS` as `DOC_CHECKS`/
`CODE_CHECKS`/`CLI_CHECKS`/`WIZARD_BASE_CHECKS` with owners
`scripts/documentation-audit.sh`, `.github/workflows/pages.yml:build`, etc.

**DISCREPANCY D (blocker).** The real `CI_CHECKS` (`pr-gate-plan.py:632-635`) is:
```
{'id':'pr-planner-tests',  'command':'python3 -m unittest scripts/test_pr_gate_plan.py'},
{'id':'ci-policy',         'command':'bash scripts/ci-workflow-policy.sh'},
{'id':'ci-executor-verifier-tests', 'command':'PYTHONPATH=scripts python3 -m unittest scripts/test_pre_release_resume.py scripts/test_evidence_api_transport.py scripts/test_select_evidence_attempts.py'},
```
The draft's `CI_CHECKS`/`DOC_CHECKS` do not exist as documented — there is no
`DOC_CHECKS` array in the file (only `CODE_CHECKS`, `CLI_CHECKS`,
`WIZARD_BASE_CHECKS`, `INSPECT_CHECKS`, `CAPABILITY_CHECKS`), no
`documentation-audit`, `mkdocs-en-strict`, `pages-candidate-build`, `cli-guided-tests`,
`cli-vet`, `cli-build`, or `cli-gofmt`. The draft's section 3 is fabricated.

`required_checks` semantics (`pr-gate-plan.py:758`):
```
require([{'id': check} for check in record['required_checks']])
```
Every entry must be a **check id** present in the gate catalog; `require()` adds
each to `validations`, and the final result lists
`required_validation_checks`. A `required_checks` value that is not an id (e.g.
a full command or a non-existent id) yields a missing validation entry and
blocks CI. `repro_binding_trusted_base.py:79` uses `[c['id'] for c in
planner.CI_CHECKS]` — the correct pattern. The draft must pin
`required_checks` to the real `[`pr-planner-tests`,`ci-policy`,`ci-executor-verifier-tests`]`
ids, not to the invented array.

## (5) `validated_diff` required fields (draft section 3 list)

The draft's `validated_diff` field list (`draft-record-pr.md:102-110`) is
**correct** against `pr-gate-plan.py:22-50`: `broad_required` bool, `changes`
array of objects, `reasons` array of `{code,message}` objects,
`target_base_sha`/`head_sha`/`merge_base_sha` 40-hex lowercase,
`integration_sha` null-or-40-hex. No discrepancy here.

## (6) Other

- Draft cites `load_binding_qualification_record` at `repro_binding_trusted_base.py:34-96`
  (`:170`); the real loader is `pr-gate-plan.py:441-492`. The repro's
  `repro_binding_trusted_base.py:68-80` `record()` builds a *correct* record
  (`files`/`file_hashes`/`required_checks` present), which is the authoritative
  shape the draft should mirror — the draft contradicts its own repro.
- Draft blob ids for map file (`dd3f360a`) match e2060ea0; the loader blob id
  (`09b480710`) does not.

## Conclusion

Four blockers, all of which would make the future record PR fail CI or be
unverifiable:

1. **A** — skeleton omits required `files`/`file_hashes`; `reviewed_head` is
   nested under `pins` not top-level.
2. **B** — loader blob id `09b480710` is a stale revision, not e2060ea0's
   `c199302e`.
3. **C** — `scripts/ci-binding-qualifications.json` classifies as
   `ci-executor-unresolved` (the `scripts/ci-` branch at
   `pr-gate-plan.py:1074`), NOT the prose bucket; the map `areas` are unused;
   a develop-only policy PR fails closed, not exempted.
4. **D** — documented `CI_CHECKS`/`DOC_CHECKS` are fabricated; real ids are
   `pr-planner-tests`, `ci-policy`, `ci-executor-verifier-tests`.

`validated_diff` field list (5) and the `git cat-file blob | sha256sum` command
form (2 command shape) are correct. Fix A–D before this record PR is authored
or it will fail CI.

Q16-RECORD-DRAFT-DONE
