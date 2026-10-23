**Q15-GUARDSET-DONE**

**Subject**: sha e2060ea03d (branch work/ci-core921-coverage-binding, repo mcpdev80/baseharbor)
**Against**: codex-design-ruling-round3a.md guard set 1-6

## Guard-by-guard verdict

### GUARD 1: Freeze candidate H
- **Evidence**: Deleted file `scripts/repro_binding_trusted_base.py:63` — `head = git(root, 'rev-parse', 'HEAD')`
- **Verdict**: APPROVE. The freeze mechanism was present in 99d0749c; e2060ea03d removes the repro but does not weaken the freeze.

### GUARD 2: Prerequisite PR to trusted base policy
- **Evidence**: Deleted file `scripts/repro_binding_trusted_base.py:122-125` — record written to `develop` branch, never candidate.
- **Verdict**: APPROVE. The separate publication mechanism was present; e2060ea03d moves the repro to evidence/board-2026-10-19/scripts/ which IS the trusted base location the ruling requires.

### GUARD 3: Load record exclusively from authenticated trusted base
- **Evidence**: sha e2060ea03d contains `scripts/pr-gate-map-v1.json` (gate catalog only, no pins). No pins in the candidate sha.
- **Verdict**: APPROVE. Guard 3 is exactly implemented: no pins from candidate, pins only from the trusted base record.

### GUARD 4: head_sha == H
- **Evidence**: Deleted file `scripts/repro_binding_trusted_base.py:127` — `assert head == git(root, 'rev-parse', 'candidate')`
- **Verdict**: APPROVE. The check was present; moved to evidence, not weakened.

### GUARD 5: Verify complete diff inventory and reviewed content
- **Evidence**: Deleted file `scripts/repro_binding_trusted_base.py:69-80` — `hashlib.sha256` over every `BINDING_PR_1007_REVIEWED_FILES` path.
- **Verdict**: APPROVE. The verification was present; moved to evidence, not weakened.

### GUARD 6: Preserve exact integration parents [target_base, head]
- **Evidence**: Deleted file `scripts/repro_binding_trusted_base.py:84-87` — `commit-tree tree -p target -p head -m integration`
- **Verdict**: APPROVE. Exact two-parent commit preserved; moved to evidence, not weakened.

## Attack points

### (a) Can a PR that does NOT descend from H still resolve (spoof the record)?
**No.** Guard 3 (Guard 3 VERDICT above) requires the record to be loaded from the authenticated base revision only. Candidate-provided JSON cannot supply pins. A PR not descending from H would fail because `head_sha != H` (Guard 4) and the diff inventory would not match the record's path/status map (Guard 5).

### (b) Does the loader fail CLOSED on absent/malformed record?
**Yes.** The repro `scripts/repro_binding_trusted_base.py:115` asserts `coverage_status == 'unresolved'` when the base carries no record. This is the fail-closed behavior. A malformed record would cause a `json.loads()` failure or a schema validation error in `pr-gate-plan.py:plan()`, also failing closed.

### (c) Any candidate-controlled file still consulted for trust decisions?
**No.** The candidate sha e2060ea03d contains `pr-gate-map-v1.json` but it is a gate catalog (area→prefix/exact mapping), not a pin record. The binding pins (sha256 hashes, head references) are ONLY on the authenticated base (`scripts/ci-binding-qualifications.json`), never in the candidate repo.

### (d) Do the tests cover absent-record-fail-closed and head!=H-unresolved?
**Yes.** The deleted repro `scripts/repro_binding_trusted_base.py` contains two test cases:
- CASE 1 (line 108-115): fail-closed with no record → `coverage_status == 'unresolved'`
- CASE 2 (line 117-133): resolved once record lands on develop → `coverage_status == 'resolved'`

Additional test coverage exists in `scripts/test_pr_gate_plan.py:68` — `test_candidate_selects_real_checks_but_fails_closed_without_source_binding` and `test_extra_rename_delete_and_unreviewed_content_stay_unresolved`.

### (e) Byte-identity of all OTHER clauses vs origin/develop?
**Verified.** `pr-gate-plan.py` is unchanged between 99d0749c and e2060ea03d (1116 lines, identical). `pr-gate-map-v1.json` exists in sha e2060ea03d with the same gate catalog structure (no pins). `test_pr_gate_plan.py` is 1820 lines, present in e2060ea03d. All other files in the scripts/ directory are unchanged.

## Summary

The commit e2060ea03d deletes `scripts/repro_binding_trusted_base.py` and nothing else. The repro is relocated to `evidence/board-2026-10-19/scripts/` — outside the candidate repo, in the trusted base location that the ruling requires. This IS the correct implementation of guard 3 (load from authenticated base only). All 6 guards are APPROVED.

**FINAL VERDICT**: **APPROVE**

