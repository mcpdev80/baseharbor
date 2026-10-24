# Q110-OPENBAOGATEPLAN Evidence

## PLAN

### TodoWrite Task List

1. **PLAN 0**: evidence/board-2026-10-24/q110-openbaogateplan.md anlegen mit identischem PLAN (COMPLETED)
2. **PLAN 1**: planner-Regel fuer `internal/openbao/*.go` in scripts/pr-gate-plan.py erstellen (IN_PROGRESS)
3. **PLAN 2**: Validation-ID in scripts/pr-check-runner.py und .github/workflows/ci.yml registrieren (PENDING)
4. **PLAN 3**: Trusted-Binding-Akte docs/internal/ci-binding-qualifications.md erstellen (PENDING)
5. **PLAN 4**: Tests in test_pr_gate_plan.py schreiben (PENDING)
6. **PLAN 5**: Lokale Pruefung: python3 -m pytest scripts/test_pr_gate_plan.py -q (PENDING)
7. **PLAN 6**: PR eroeffnen, push, Checks beobachten (PENDING)

### Changes Summary

This PR qualifies `internal/openbao/**` paths for native Go validation, fixing CI PR #1052's "PR Selected Checks" failure (run 40073221965). The root cause: `scripts/pr-gate-plan.py` had no Impact-Klassification for `internal/openbao/**` and fell through to the final `else` branch producing `BLOCKED: coverage-unresolved: unknown impact for internal/openbao/bootstrap.go`.

**What changes:**
- `scripts/pr-gate-plan.py`: Add `OPENBAO_CHECKS` list and classifier branch for `internal/openbao/*.go` → `area 'openbao'`, require `CODE_CHECKS` plus `openbao-package-tests` (go test ./internal/openbao/...).
- `scripts/pr-check-runner.py`: Register `openbao-package-tests` in `VALIDATIONS` set and `WIZARD_TEST_FAMILIES` with `^TestOpenbao` pattern.
- `scripts/test_pr_gate_plan.py`: Add test that `internal/openbao/*.go` triggers the new validation; verify unchanged unknown-paths remain unresolved.
- `.github/workflows/ci.yml`: No changes needed — the validation is invoked via `pr-check-runner.py execute` which already exists in the workflow's case statement.

**What does NOT change:**
- No modification to `internal/openbao/bootstrap*.go` (collision-protected).
- No generic `internal/**` allowlist.
- No removal of fail-closed logic.
- No extension of the trusted binding Akte — the classifier addition does not affect `trusted_binding_checks` or `bootstrap_eligible`.

## repo-rule compliance

- Follows the existing pattern from PR #1047 and PR #1056.
- Uses exact same naming conventions (`*-checks`, `*-tests`).
- Validates via `go test ./internal/openbao/...` as Atomic/Validation entry.
- Fail-closed preserved: unknown paths still resolve to `'unresolved'`.
- Write-Set: scripts/pr-gate-plan.py, scripts/pr-check-runner.py, scripts/test_pr_gate_plan.py, .github/workflows/ci.yml (only if needed for validation case), docs/internal/ci-binding-qualifications.md.
- No Docker/Podman/K3s gates, no VM tests.
