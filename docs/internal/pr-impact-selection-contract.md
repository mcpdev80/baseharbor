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

## Narrow source classification (review correction)

Only the audited guided read-adapter CLI files are treated as statically
resolved; even those require the real `cmd/baha` package's focused tests,
`go vet`, and `go build`. All other `cmd/baha/*.go`, including
backup/restore, lifecycle, mutable commands, and newly introduced files, return
`coverage_status: unresolved` with an explicit native qualification
requirement. CLI checks alone do **not** resolve their runtime impact.

Only audited planning scripts and the Pages documentation workflow are eligible
for resolved CI/documentation-only classification. Runtime execution, GC,
artifact lifecycle, release and evidence workflows, plus related CI executor
scripts, require the existing policy/planner/verification regressions **and**
remain `unresolved` pending justified native qualification. The planner does
not infer native impact from arbitrary workflow YAML. No automatic full matrix.

## Additional package/runtime boundary

The only `internal/capability/` resolved paths are the audited `domain_test.go` and `integration_contract_test.go` conformance tests. These require the package's actual `go test ./internal/capability/... -count=1`, as well as existing contract checks. All capability implementation/registry/provider paths and all `internal/application/` paths remain explicitly `unresolved` pending scoped impact qualification. The previously broad `deploy/docker/**` lifecycle-only resolution is removed entirely: arbitrary deployment, backup, security, or launch scripts require justified native qualification. No full-matrix fallback is introduced.

The only `internal/repositoryinspect/` resolved paths are the audited conformance
test paths (`workload_source_parity_test.go`, `inspect_test.go`) and the exact
reviewed production path `inspect.go` (core-750 source-neutral summary
projection). These resolve to the focused repositoryinspect package tests,
`go vet ./cmd/baha`, `go build ./cmd/baha`, and the contract/application
`go test` superset with **zero** selected release gates. Every other
`internal/repositoryinspect/*.go` production or test file stays
`unresolved` pending scoped native qualification; no blanket package
resolution is claimed.

The only `internal/runtime/resolver/` resolved paths are the audited
`resolver.go` production source and its `resolver_test.go` conformance test
(core-1220 resolver routing qualification). These resolve to the focused
resolver package tests, `go vet ./internal/runtime/resolver`,
`gofmt ./internal/runtime/resolver`, and the `cmd/baha` consumer build with
**zero** selected release gates. Every other
`internal/runtime/resolver/*.go` production or test file stays
`unresolved` pending scoped native qualification; no blanket package
resolution is claimed. Both the inspect and resolver narrow lanes are exact
reviewed-path allowlists, never prefix or name heuristics.
