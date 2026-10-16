# PR runtime request handoff for #896 (v1)

This is the narrow integration boundary between #893 and #896. The producer is
`scripts/pr-runtime-request.py` at the reviewed #930 SHA, using trusted
`pr-gate-plan/v1`, the actual versioned release requirements and verified
immutable Demo, Console and Connector pins.

- Each `baseharbor.pr-runtime-request/v1` object identifies one selected,
  existing Docker/Podman gate and exact target base, head, merge base,
  integration and consumer commits. It is **not** authorization to execute
  PR head code or proof of a completed test.
- #896 owns admission, runner tenancy and isolation: one disposable target per
  PR/run/attempt/gate, unique namespace/ports/volumes/networks and all provider,
  secret and recovery state, plus bounded cleanup even on failure/cancellation.
  Do not use privileged self-hosted access or secrets with untrusted PR code.
  Forks remain unprivileged. Independent lanes must not cancel each other.
- The runner result is **not** a free-form PR-controlled success JSON.
  #893 consumes job conclusion and authenticated existing verifier evidence:
  exact request input identity, run/attempt, repository/workflow provenance,
  artifact digest, selected gate ID and result. Missing, stale, failed,
  skipped, cancelled, timed-out or unverifiable evidence blocks
  `PR Selected Checks`.
- Any candidate binary/image must be built or proven reused for the exact
  integration SHA and immutable dependencies. Cache is not evidence.
  Candidate preparation must not depend on unrelated docs/snapshot checks.
- #896 returns isolated runner helpers, focused negative/isolation fixtures and
  two small Docker/Podman pilot lanes with cleanup evidence in a separate PR.
  It must not edit #893's `ci.yml` or its final-check implementation.
  #893 then integrates authenticated evidence and qualifies the targeted
  Backup/Restore path. Until then required runtime gates remain blocked.

This contract adds no runner, gate, evidence authority or execution permission.
