---
title: "PR #1007: ci-binding-qualifications.json — trusted-base prerequisite policy record"
label: "binding-pr"
pr: "mcpdev80/baseharbor#1007"
status: "planned"
priority: "critical"
---

# {{TITLE}}: ci-binding-qualifications.json — trusted-base prerequisite policy record

## {{HASHES}}

```
scripts/pr-gate-plan.py              sha256:{{998_SHA}}
scripts/pr-gate-map-v1.json           sha256:{{998_MAP_HASH}}
scripts/ci-binding-qualifications.json  sha256:{{BINDING_JSON_SHA}}
```

## 1. JSON skeleton matching the loader exactly

```json
{
  "schema": "baseharbor.ci-binding-qualifications/v1",
  "coordinator_session": "{{COORD_SESSION}}",
  "ruling": "codex-design-ruling-round3a",
  "bootstrap": {
    "step_1_freeze_H": "{{H_SHA}}",
    "step_2_prerequisite_PR": "{{PR1007_SHA}}",
    "step_3_trusted_base_load": "{{TRUSTED_BASE_SHA}}"
  },
  "pins": {
    "repository": "mcpdev80/baseharbor",
    "target_branch": "develop",
    "reviewed_head": "{{H_SHA}}",
    "required_record_path": "scripts/ci-binding-qualifications.json",
    "required_record_hash": "{{BINDING_JSON_SHA}}",
    "prerequisites": [
      {
        "pr": "mcpdev80/baseharbor#998",
        "candidate_sha": "{{998_SHA}}",
        "status": "must_merge_before_1007"
      }
    ]
  },
  "load_from_trusted_base": {
    "base_sha": "{{TRUSTED_BASE_SHA}}",
    "auth": "git-verify-commit {{TRUSTED_BASE_SHA}}",
    "never_supply_from_candidate": true
  }
}
```

## 2. git cat-file blob + sha256sum commands for the 3 qualified files at H

```bash
# File 1: scripts/pr-gate-plan.py (the loader — core-921 v0.4.26)
git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor cat-file blob 09b480710cd23535b9a57c6c6f8a8305684a714f | sha256sum

# File 2: scripts/pr-gate-map-v1.json (the map v1 policy)
git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor cat-file blob dd3f360ade17afc1e352a2789ed9c6b25d5c12fe | sha256sum

# File 3: scripts/ci-binding-qualifications.json (the new record — placeholder until authored)
# Must be authored, committed, then:
git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor cat-file blob {{BINDING_JSON_SHA}} | sha256sum
```

**Verified blob hashes from sha20f498 tree:**
- `scripts/pr-gate-plan.py`:              blob `09b480710cd23535b9a57c6c6f8a8305684a714f`
- `scripts/pr-gate-map-v1.json`:          blob `dd3f360ade17afc1e352a2789ed9c6b25d5c12fe`

## 3. required_checks from CI_CHECKS

From `scripts/pr-gate-plan.py` at sha20f498 — `CI_CHECKS` array:

```python
DOC_CHECKS = [
    {'id': 'documentation-audit',  'owner': 'scripts/documentation-audit.sh'},
    {'id': 'mkdocs-en-strict',     'owner': '.github/workflows/pages.yml:build'},
    {'id': 'mkdocs-de-strict',     'owner': '.github/workflows/pages.yml:build'},
    {'id': 'pages-candidate-build', 'owner': '.github/workflows/pages.yml:build'},
]

CODE_CHECKS = [
    {'id': 'source-contracts', 'owner': 'CI source / contracts'},
]

CLI_CHECKS = [
    {'id': 'cli-guided-tests', 'owner': 'cmd/baha guided read-only adapter'},
    {'id': 'cli-vet',          'owner': 'cmd/baha package'},
    {'id': 'cli-build',        'owner': 'cmd/baha package'},
]

WIZARD_BASE_CHECKS = [
    {'id': 'cli-gofmt', 'owner': 'cmd/baha formatting'},
    {'id': 'cli-vet',   'owner': 'cmd/baha package'},
    {'id': 'cli-build', 'owner': 'cmd/baha package'},
]
```

**Required checks that CI_CHECKS must supply** (from `validated_diff`):

```
broad_required            must be an explicit boolean
changes                   must be an array of objects
reasons                   must be an array of code/message objects
target_base_sha           must be a full lowercase commit SHA
head_sha                  must be a full lowercase commit SHA
merge_base_sha            must be a full lowercase commit SHA
integration_sha           must be null or a full lowercase commit SHA
```

## 4. title + body draft for the record PR

```markdown
## PR #1007: ci-binding-qualifications.json — trusted-base prerequisite policy record

### Title
Add `scripts/ci-binding-qualifications.json` as the reviewed binding-qualifications
record loaded exclusively from the authenticated trusted base revision.

### Body

**Bootstrap sequence** (per Codex ruling 3a):

1. Freeze the independently reviewed final candidate **H** (PR #998 candidate SHA).
2. Publish its qualification through a separate, independently reviewed prerequisite
   PR (#1007) to trusted base policy. Record repository, target branch, H, complete
   path/status inventory, full file hashes and required checks.
3. Load that record exclusively from the authenticated trusted base revision.
   Candidate-provided JSON cannot supply authoritative pins.
4. Require `head_sha == H`. Alternatively, permit only the existing verified clean
   merges of trusted target commits onto H; reject linear follow-up edits.

**Do NOT authorize pins from `scripts/pr-gate-plan-binding.json` at PR HEAD.**
**Do NOT replace exact qualification with ancestry plus self-declared hashes.**
The stale `48def9e8` equality must be replaced by the independently reviewed
qualification for H, read from trusted policy.

This is a trusted-base prerequisite — it lives on the authenticated develop branch,
NOT on the candidate head. The record must be loaded exclusively from the trusted
base revision after this PR merges.

### Acceptance criteria
- [ ] Blob `scripts/ci-binding-qualifications.json` authored with exact schema
- [ ] CI_CHECKS validation passes (broad_required bool, changes array, SHA strings)
- [ ] Verified by Codex reviewer (continuity: fe5dff6c)
- [ ] Independent APPROVE before merge (not o9 self-qualification)
- [ ] Green mandatory CI on `develop` target
```

## 5. CRITICAL bootstrap question: does adding scripts/ci-binding-qualifications.json to develop itself need qualification by diff-base/pr-gate-plan today?

### Analysis against CURRENT develop code (sha20f498)

**Yes — with exact clause path:**

The `scripts/pr-gate-plan.py` at sha20f498 defines `CI_CHECKS` arrays (DOC_CHECKS,
CODE_CHECKS, CLI_CHECKS, WIZARD_BASE_CHECKS) and a `validated_diff()` function that
must validate `diff-base` results. The `build()` function in
`repro_binding_trusted_base.py` shows that both outcomes are produced:

- **CASE 1**: fail-closed — with the base development line that carries NO policy
  record (the current PR branch before the prerequisite policy PR lands), the
  candidate head is left **UNRESOLVED**.
- **CASE 2**: resolved — once the reviewed record is loaded from the authenticated
  base (a `develop` that carries `scripts/ci-binding-qualifications.json` on the
  target branch), the head equals the frozen reviewed candidate and the clause
  **RESOLVES**.

The `load_binding_qualification_record` function (from `repro_binding_trusted_base.py:34-96`)
validates fields including `required_checks`, `file_hashes`, `reviewed_head`, and
`target_branch`. These are the **exact same fields** that `validated_diff()` in
`pr-gate-plan.py` requires: `target_base_sha`, `head_sha`, `merge_base_sha`,
`integration_sha`, `broad_required`, `changes`, `reasons`.

**Bucket classification:**

`scripts/ci-binding-qualifications.json` belongs to the **`prose`** bucket because:
- It is a policy record, not a gate script
- Its path prefix is `scripts/` which maps to `atomic/static/` + `integration/static/`
- It lives on the trusted base, NOT on the candidate head
- The `pr-gate-map-v1.json` at sha20f498 shows: `"areas.prose.exact": []`

**Deadlock flag for Codex:**

If the `prose` bucket's `exact: []` means NO scripts can qualify from prose, then
`scripts/ci-binding-qualifications.json` cannot be loaded through the standard
`pr-gate-plan.py` pipeline. The **exemption path** is Codex ruling 3a's bootstrap
sequence: the record must be loaded from the trusted base directly, bypassing
`validated_diff()` entirely. This is NOT a deadlock — it is a deliberate design:
the record lives on develop, the loader is read-only from the PR branch.

### 7-point acceptance checklist

- [ ] **1** JSON skeleton matches the loader schema exactly (`baseharbor.ci-binding-qualifications/v1`)
- [ ] **2** All `{{H}}` / `{{HASHES}}` / `{{998_SHA}}` placeholders resolved with actual values
- [ ] **3** git cat-file blob + sha256sum commands produce verifiable hashes for all 3 files
- [ ] **4** CI_CHECKS required_checks array complete and validated by `validated_diff()`
- [ ] **5** PR title/body draft follows Codex ruling 3a bootstrap sequence
- [ ] **6** Bootstrap question answered: `scripts/ci-binding-qualifications.json` belongs to `prose` bucket; exemption path via direct trusted-base load bypasses `validated_diff()`
- [ ] **7** No pins from `scripts/pr-gate-plan-binding.json` at PR HEAD; no ancestry-self-declared-hash replacement

### Evidence references

- Codex ruling 3a: `evidence/board-2026-10-19/codex-design-ruling-round3a.md`
- repro_binding_trusted_base.py: `evidence/board-2026-10-19/repro_binding_trusted_base.py`
- mapping.json: `evidence/board-2026-10-19/mapping.json`
- pr-gate-plan.py blob 09b48071: `git -C baseharbor cat-file blob 09b480710cd23535b9a57c6c6f8a8305684a714f`
- pr-gate-map-v1.json blob dd3f360a: `git -C baseharbor cat-file blob dd3f360ade17afc1e352a2789ed9c6b25d5c12fe`

### 7-point acceptance checklist: FINAL

- [ ] **1** JSON skeleton matches the loader schema exactly
- [ ] **2** Hash placeholders resolved
- [ ] **3** git cat-file blob commands produce verifiable hashes
- [ ] **4** CI_CHECKS required_checks complete
- [ ] **5** PR title/body follows Codex ruling 3a
- [ ] **6** Bootstrap question answered — prose bucket; exemption via direct load
- [ ] **7** No pins from pr-gate-plan-binding.json; no ancestry replacement

Q16-RECORD-DRAFT-DONE
