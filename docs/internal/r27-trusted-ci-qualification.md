# R27 independent CI qualification contract

This document is a review input, **not** an allowlist, gate mapping, or
authorization to accept a changed PR. It must be reviewed against the current
trusted planner on `develop` before any policy implementation.

## Candidate

- Product draft: #1048, branch `work/r27-evidence-archive-isolated-20261023`.
- Changed module/test pairs: `r27_archive_publisher`,
  `r27_archive_resolver`, `r27_evidence_receipt`,
  `r27_evidence_coverage`, `r27_verified_coverage` (all under
  `scripts/` and `scripts/tests/`).
- Baseline: `develop`; policy must come from the authenticated base,
  not from the candidate PR.

## Impact inventory

| Pair | Security-sensitive impact | Required negative qualification |
| --- | --- | --- |
| archive_publisher | publication and immutability of evidence | tampering, wrong run, overwrite, traversal, failed upload |
| archive_resolver | selecting archived evidence | stale candidate, policy mismatch, duplicate/ambiguous receipts |
| evidence_receipt | receipt signature and key custody | forged MAC, duplicate keys, wrong candidate/policy, weak key |
| evidence_coverage | gate reuse decisions | caller-supplied authenticated flag, missing/failed gates, mandatory smoke |
| verified_coverage | gate provenance verification | forged MAC, wrong CI identity, stage, policy, artifact, malformed JSON |

## Qualification sequence

1. Independently review all ten exact paths and their actual diff. Do not
   qualify based on names, branch labels, or a changed planner from the PR.
2. Execute the five focused Python unit suites on the exact candidate commit.
   Test discovery must include `scripts/tests/test_r27_*.py`, and report
   discovered test counts and failing cases. Existing CI selected checks alone
   do not establish these tests ran.
3. Inspect trusted issuer boundaries: the signing key must never be available
   to untrusted PR code; CI run/attempt/job identities must be authenticated
   from trusted GitHub context and independently checked before signing.
4. Propose **exact-path** mappings and targeted checks in a separate policy
   change. A path mapping is not evidence that the checks passed. Reject
   unknown paths and changed policy files until independently qualified.
5. Validate policy planner tests, selected executor checks and finalcheck
   against a fixed candidate SHA. Capture CI run, attempt, job, policy digest,
   selected gates and the result for each. Release requires artifact-bound
   smoke proof and independent issuer approval.

## Fail-closed contract

`coverage-unresolved` must remain blocking until all requirements above are
satisfied. Never replace it with a broad script wildcard, synthetic PASS,
caller-supplied `authenticated=True`, or a trusted signing key in a PR
workflow. No self-qualification, merge, release, or promotion is authorized.
