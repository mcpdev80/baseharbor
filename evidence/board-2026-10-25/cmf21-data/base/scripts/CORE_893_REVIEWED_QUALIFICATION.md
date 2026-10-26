# core-893 independent, immutable coverage qualification

Reviewed candidate: product PR #930 at `b0acfc32ddf4d940294a43f98db4618d3aba83cd`.
Trusted original merge-base: `14d864dfd38801a56e54901ea520f3b02db134b2` (#942).
Authority: trusted planner in the PR target-base checkout, never a policy file
or qualification record read from the PR head. The exact reviewed commit
cryptographically binds its complete tree and all eighteen changed paths.
Qualification also requires the trusted diff resolver to report a valid
integration commit and non-broad changes. Any different head, baseline,
additional/renamed/missing file or invalid integration blocks. This is
**not** a reusable allowlist or permission for future CI changes.

## Risk to qualification mapping

- `pr-planner-tests`: trusted diff, required-check selection, adversarial
  missing/changed-path and immutable-head regressions. Does not prove runtime.
- `ci-policy`: workflow security surface, permissions, immutable helper
  selection and YAML policy. Does not prove evidence authenticity.
- `ci-executor-verifier-tests`: pre-release resume, evidence transport,
  exact run/attempt selection and canonical evidence verification.
- Mandatory `PR · Focused Python`: executor `test_pr_check_runner.py`,
  `test_pr_runtime_request.py`, `test_pr_selected_runtime_matrix.py`,
  `test_pr_gate_dependencies.py`; authenticated results
  `test_pr_authenticated_results.py`; final checker
  `test_pr_required_final.py`; original gate evidence
  `test_pr_verified_atomic_evidence.py`; plus diff, planner, pin and
  resume regression suites. These prove fail-closed handling, not native success.
- `integration/static/public-contracts`: genuine existing integration owner,
  immutable Core/Demo inputs, original signed-by-provenance Actions artifact
  and authenticated static final-check ingestion.
- `atomic/docker/backup-restore`: end-to-end native Docker matrix dispatch,
  exact candidate, Demo pin, runner isolation, cleanup and canonical original
  artifact authentication on this candidate.
- `atomic/podman/backup-restore`: equivalent rootless Podman execution,
  including isolation, cleanup and immutable artifact provenance.

Both native gates must pass on the new PR integration SHA; prior receipts
from different source/integration commits do not qualify this candidate.
Runner isolation additionally uses the qualified core-896 interface from
the trusted base and its `test_pr_runner_isolation.py` fixture, executed
by `PR · Runner Interface` before native gates.

This acceptance is only a scoped qualification decision. Merge requires
independent review, exact-source GitHub Actions evidence for the three gates,
successful final selected-check enforcement, and no unresolved coverage.
