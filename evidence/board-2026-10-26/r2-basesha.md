# R2 Base-SHA Resolution Evidence

## PLAN

### Objective
Replace `base_ref`-als-BASE_SHA in `receipt-post-run.yml:48` with ECHTE SHA-Auflösung des PR-base from `github.event.workflow_run.base.sha`, exactly as the P3R Review-Vorschlag describes (P3R-reviews-l5.md Abschnitt (3)).

### Scope
- File: `.github/workflows/receipt-post-run.yml:48`
- File: `scripts/receipt_tasklink.py` (no changes needed — already validates sha40)
- File: `scripts/test_receipt_tasklink.py` (extend with base-Auflösung tests)

### Changes
1. **YML Hook (line 48):** Replace `BASE_SHA: ${{ github.base_ref || 'main' }}` with `BASE_SHA: ${{ github.event.workflow_run.base.sha || github.event.workflow_run.base_ref }}`
2. **Tests:** Add 6 new test cases in `TestBaseShaResolution` class to verify base-Auflösung with fake API
3. **All 16 tests pass** (10 original + 6 new)

### File:Line References
- `receipt-post-run.yml:48` — BASE_SHA resolution change
- `test_receipt_tasklink.py:245-328` — TestBaseShaResolution class with 6 test cases

## Evidence

### P3R-Befund (3) addressed
- **Original Problem:** `base_ref`-als-BASE_SHA on line 48 was unreliable in workflow_run context
- **Solution:** Use `github.event.workflow_run.base.sha` (40-char hex) as primary, `base_ref` as fallback
- **Fail-closed:** sha40() rejects 'main' or any non-40-char hex — ensures real SHA required

### Test Results
```
Ran 16 tests in 2.300s
OK
```

All 6 Receipt-Suiten gruen:
1. test_produce_plan_accepts_real_base_sha
2. test_cli_accepts_real_base_sha
3. test_fail_closed_rejects_non_hex_base_sha
4. test_fail_closed_rejects_short_base_sha
5. test_sha40_validates_all_sha_fields
6. test_produce_plan_with_mixed_valid_inputs

### YML Syntax
- Verified clean with pyyaml safe_load

## repo-rule compliance
- Changes isolated to r2/ustrack worktree only
- No PR1-Branch touched
- No push performed
- Single ordinary commit as specified

## END R2-DONE
