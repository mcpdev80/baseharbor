# Q117 Takeover — #1060 PR Head 6a0d36da

## PLAN + native Todos

- [x] 0. Read ops/missions/q117.txt; `pwd -P` in worktree and root
- [ ] 1. Map exact status: worktree/branch/HEAD, diff(develop, 1c2f1fc4), diff(develop, 6a0d36da), relationship 1c2f1fc4 vs 6a0d36da (merge-base, left-right, file diffs)
- [ ] 2. Integrity decision: 6a0d36da adds W-PR content (ci.yml openbao case) and fixes pr-gate-plan.py binding clause; must integrate into worktree branch
- [ ] 3. Close review findings: handler, command selection, binding clause, record hashes
- [ ] 4. Run minimal local checks: pytest on affected test files
- [ ] 5. Commit + push to PR branch work/v0.4.26-openbao-gateplan
- [ ] 6. Observe CI checks max 25min, document results
- [ ] 7. Report END Q117-...

---

## 1. Status mapping

### 1.1 Worktree / branch / HEAD

- Worktree: `/home/marcel/dev/baseharbor-ecosystem/q114-fix1060`
- Branch: `q114-fix1060`
- HEAD: `1c2f1fc4 fix(ci): correct binding clause to read from HEAD and match actual 3-file diff`
- Parent: `9ec25282 ci(gate-plan): add Trusted-Binding-Akte entry for #1060`
- Working tree clean: no uncommitted changes

### 1.2 Relationship between 1c2f1fc4 and PR-head 6a0d36da

- merge-base(1c2f1fc4, 6a0d36da) = `9ec2528253e1d91c48f81985f7c696a663a29fd6` (the parent commit)
- 6a0d36da is exactly **1 commit** beyond the common ancestor 9ec25282
- 1c2f1fc4 is exactly **1 commit** beyond the common ancestor 9ec25282
- Both branches diverge from the same ancestor: 1c2f1fc4 and 6a0d36da are **sibling commits** off 9ec25282
- 6a0d36da was authored earlier (12:33 UTC) than 1c2f1fc4 (13:34 UTC)
- 6a0d36da has 5 changed files (ci.yml, ci-binding-qualifications.md, pr-check-runner.py, pr-gate-plan.py, test_pr_gate_plan.py)
- 1c2f1fc4 has 3 changed files (ci-binding-qualifications.md, pr-check-runner.py, pr-gate-plan.py)
- The diff 1c2f1fc4..6a0d36da changes 4 files: ci.yml, ci-binding-qualifications.md, pr-gate-plan.py

### 1.3 What 6a0d36da adds beyond 1c2f1fc4

1. **ci.yml:719**: Adds `openbao-package-tests) go test ./internal/openbao/... -count=1 ;;` case
2. **pr-gate-plan.py:638**: Changes `diff['head_sha']` → `diff['target_base_sha']` for reading binding qualifications
3. **pr-gate-plan.py:654-656**: Changes expected_files from ci-binding-qualifications.md to test_pr_gate_plan.py
4. **test_pr_gate_plan.py:2186-2229**: Adds 38 lines of `OpenBaoValidationTests` class
5. **ci-binding-qualifications.md:12-14**: Updated to reference test_pr_gate_plan.py in files/hashes

### 1.4 What 1c2f1fc4 has that 6a0d36da does NOT have (or differs on)

- 1c2f1fc4's pr-gate-plan.py uses `diff['head_sha']` (6a0d36da uses `diff['target_base_sha']`)
- 1c2f1fc4's expected_files includes ci-binding-qualifications.md (6a0d36da uses test_pr_gate_plan.py)
- 1c2f1fc4 does NOT have the ci.yml openbao-package-tests case (6a0d36da adds it)
- 1c2f1fc4 does NOT have the OpenBaoValidationTests in test_pr_gate_plan.py (6a0d36da adds them)

---

## 2. Integrity decision

**6a0d36da is not a superset of 1c2f1fc4 — they are divergent sibling commits.**

- 6a0d36da is the original PR-head commit with ci.yml openbao case, OpenBaoValidationTests, and a binding clause fix using `target_base_sha`
- 1c2f1fc4 is a later correction that changed the binding clause to read from HEAD and match the actual 3-file diff, but dropped the ci.yml case and test additions

**Decision: merge forward from 1c2f1fc4 to incorporate 6a0d36da's changes as additional commits**, preserving both:
1. The ci.yml openbao-package-tests case (W-PR content per q117#7)
2. The OpenBaoValidationTests regression coverage
3. The binding clause fix from 1c2f1fc4 (reading from HEAD with actual 3-file diff)
4. The corrected expected_files from 6a0d36da (test_pr_gate_plan.py)

This requires a single forward correction commit that:
- Keeps 1c2f1fc4's binding clause (`diff['head_sha']`) — it was the correct fix
- Adds 6a0d36da's ci.yml openbao-package-tests case
- Adds 6a0d36da's OpenBaoValidationTests  
- Uses 6a0d36da's corrected expected_files (test_pr_gate_plan.py)
- Updates the binding record hashes to match the resulting candidate HEAD

**BLOCKED: no creative rework** — exactly document what needs to change, implement the forward correction, then push.

---

## 3. Implementation plan for the forward correction commit

The next commit on q114-fix1060 must incorporate 6a0d36da's unique content while preserving 1c2f1fc4's fixes:

### 3.1 .github/workflows/ci.yml
Add at line 719: `openbao-package-tests) go test ./internal/openbao/... -count=1 ;;`

### 3.2 scripts/pr-gate-plan.py  
- Keep `diff['head_sha']` (1c2f1fc4's fix) — DO NOT change to target_base_sha
- Change expected_files to include test_pr_gate_plan.py (6a0d36da's correction)
- Update binding record hash for test_pr_gate_plan.py to actual HEAD blob hash

### 3.3 scripts/test_pr_gate_plan.py
Add OpenBaoValidationTests class (38 lines from 6a0d36da)

### 3.4 docs/internal/ci-binding-qualifications.md
Update files/hashes to reference test_pr_gate_plan.py with actual blob SHA-256 of the candidate HEAD

---

END Q117-1060-STATUS-MAPPED (proceeding to integrity decision and implementation)
