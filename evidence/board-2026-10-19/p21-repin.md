# P21-REPIN — re-pin core-921 reviewed registration head 6931ba9c → 89d3ddef

- Date: 2026-10-20
- Worker: Flash (deterministic re-pin, no design discretion)
- Worktree: worktrees/ci-core921-coverage-binding, branch work/ci-core921-coverage-binding
- Base at task start: e2060ea0 (verified)
- Commit: 716b31d2 `ci(core-921): re-pin reviewed registration head to 89d3ddef`
- Remote head after push: 716b31d2 (verified via `git ls-remote origin refs/heads/work/ci-core921-coverage-binding`)

## What changed

Only `scripts/pr-gate-plan.py`. Two head constants and their surrounding comments
updated from the #998 reviewed head `6931ba9c8e604419ae0afb7ff52dbdd20da840f8`
to `89d3ddef6e77cad9b8035dce0b2a4dbe403c2951` (origin
`refs/heads/work/core-921-central-cli-mcp-registration`). The 16-path
`CORE_921_PRODUCTION_REVIEWED_FILES` map (paths + statuses) is unchanged — see
map verification below.

## Before / after constant lines

BEFORE:
```
    # delta head 6931ba9c (o8-998-delta.md). The path/status set below is the exact
    # two-dot diff origin/develop..6931ba9c for these 16 core-921 registration
    # files; the three unrelated process-doc changes in that diff (DEVELOPMENT_
    # GUIDELINES, review-protection) are NOT part of the registration bundle.
    CORE_921_PRODUCTION_REVIEWED_HEAD = '6931ba9c8e604419ae0afb7ff52dbdd20da840f8'
```
AFTER:
```
# delta head 89d3ddef (re-pin from the #998 reviewed head 6931ba9c). The
# path/status set below is the exact two-dot diff origin/develop..89d3ddef
# for these 16 core-921 registration files; the three unrelated process-doc
# changes in that diff (DEVELOPMENT_GUIDELINES x2, review-protection) are NOT
# part of the registration bundle.
CORE_921_PRODUCTION_REVIEWED_HEAD = '89d3ddef6e77cad9b8035dce0b2a4dbe403c2951'
```

BEFORE (clause comment):
```
    # Bound to the final independently re-reviewed #998 head 6931ba9c (exact
    # two-dot diff origin/develop..6931ba9c for these 16 core-921
    # registration files); an exact reviewed-file/status set on a clean
    # trusted-target merge only.
```
AFTER (clause comment):
```
    # Bound to the final independently re-reviewed #998 head 89d3ddef (re-pin
    # from the reviewed head 6931ba9c; exact two-dot diff origin/develop..89d3ddef
    # for these 16 core-921 registration files); an exact reviewed-file/status
    # set on a clean trusted-target merge only.
```

## Map verification (16-path CORE_921_PRODUCTION_REVIEWED_FILES vs real diff)

Real diff `git -C baseharbor diff --name-status origin/develop..89d3ddef` restricted
to the 16 mapped paths:

```
M	cmd/baha/app_machine_adapters.go
M	cmd/baha/app_provider_choice.go
A	cmd/baha/app_registration_921.go
M	cmd/baha/commands.go
M	cmd/baha/machine_coverage.go
M	cmd/baha/main.go
M	cmd/baha/mcp_tools.go
A	cmd/baha/registration_921_test.go
M	contracts/machine/v1/control.golden.json
M	docs/cli/command-index.md
M	docs/de/cli/command-index.md
M	docs/de/reference/cli-machine-coverage.md
M	docs/de/reference/mcp.md
M	docs/reference/cli-machine-coverage.md
M	docs/reference/mcp.md
M	internal/machine/contract.go
```

Result: all 16 paths present, all statuses (M/A) identical to the map. The map
needed NO path/status change — only head constants + hashes updated.

Full diff `origin/develop..89d3ddef` has 3 ADDITIONAL process-doc paths, all
correctly OUTSIDE the registration bundle (matches the comment):
```
A	cmd/baha/app_mcp_provider_edit_native_test.go   (test-only, not part of reviewed bundle)
M	docs/DEVELOPMENT_GUIDELINES.md                   (process-doc, excluded)
M	docs/de/DEVELOPMENT_GUIDELINES.md                (process-doc, excluded)
M	docs/internal/review-protection.md               (process-doc, excluded)
```
Note: `app_mcp_provider_edit_native_test.go` is a new test file that appeared in
the 89d3ddef delta; it is NOT in the reviewed 16-file bundle and is correctly
excluded. The bundle scope is unchanged.

## Test totals

`python3 scripts/test_pr_gate_plan.py`:
- Ran 70 tests — OK
- Ran 8 tests — OK
- Ran 7 tests — OK
- Ran 5 tests — OK
(90 tests total, all OK)

`python3 scripts/test_pr_check_runner.py`:
- Ran 18 tests — OK

## sha256

Before (baseline):
```
26d5e60e4a97f74ebcd6d369fb85d576a0d5a0166c1b5ca6afbe47a7e718a8a8  scripts/pr-gate-plan.py
```
After (fixed / committed):
```
bb86a1deb40c8e436118c86744a7765369e94143156aafbb0c87d1cee1bdbf58  scripts/pr-gate-plan.py
```

## Notes

- Remote push rejected once with non-fast-forward (local ref-cache race against
  concurrent fetch); confirmed `git ls-remote` tip was e2060ea0 and the commit
  was a direct child. Pushed with explicit refspec `716b31d2:refs/heads/...`
  — fast-forwarded cleanly (e2060ea0..716b31d2).
- Nothing else touched.
