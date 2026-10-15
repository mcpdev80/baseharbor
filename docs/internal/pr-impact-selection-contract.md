# PR impact selection: explicit coverage contract (core-941)

The #892 planner remains a **trusted-base policy** input to core-893. A PR cannot
select its own checks by changing its copy of the planner, mapping, labels or
comments. The input must remain the immutable diff-base result, preserving
`target_base_sha`, `head_sha`, `merge_base_sha`, `integration_sha`,
rename source and destination, and deletion paths.

## Output contract

`baseharbor.pr-gate-plan/v1` now adds:

- `coverage_status`: `resolved` or `unresolved`.
- `unresolved_coverage`: explicit non-empty reasons whenever coverage is unresolved.
- `selected_gates`: release gate IDs **actually required** by the scoped diff.
- `required_validation_checks`: independent required checks with named owners
  and commands; nonempty checks with zero release gates is legitimate.

`broad_required` is retained as a backwards-compatible unresolved signal,
**not** an instruction to start all release gates. A resolved documentation
plan has mandatory documentation checks and `selected_gates: []`. An invalid
diff, unknown impact or missing mapped gate produces `coverage_status:
unresolved` and is **blocking**; no empty-selection shortcut or matrix fallback
is permitted. Release requirement inventory and original runtime receipt
verification are unchanged.

## Minimal core-893 consumer changes (not implemented here)

1. Read the plan only from the pinned trusted policy base and bind immutable
   diff identities; reject schema/revision mismatch and untrusted overrides.
2. Require `coverage_status == resolved`, else produce a blocking
   `coverage-unresolved` final-check outcome listing `unresolved_coverage`.
3. Schedule every `required_validation_checks` entry and selected gate;
   zero selected gates must **not** skip or pass the mandatory validation jobs.
4. Distinguish `not-required` from `PASS` and never generate synthetic
   release-gate receipts. Any selected gate still needs original successful
   authenticated evidence, exactly as before.
5. Treat coverage marked `unresolved` as an explicit reviewer/operator
   coverage decision required in a **separate trusted policy change**; never
   self-exempt or automatically execute the complete release catalog.

The existing core-893 workflow/final-check owners must integrate and
independently review this contract before enabling it on ordinary PRs.
A green planner unit-test job does not qualify that consumer integration.
