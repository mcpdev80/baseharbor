ROLLE: REVIEW

## PLAN
- [x] 0. Read dispatch and policies; record plan. Native TodoWrite unavailable; no native update claimed.
- [ ] 1. Refresh candidate and inspect workflow dependencies, call paths and pin implications.
- [ ] 2. Run local syntax and focused suites, verify exact-head CI, record verdict and recipe.

UTC: 2026-10-29T17:03:44Z
Reviewer Codex GPT-6, independent of worker; no Horde ID exposed. Head 439b13c771ee35cd24a264eb6caf70eea9928eb5, fetched develop 64d12492c5067b119224b14a2b9c82932b91f9e7, main 115e4a73aeea8bceb9ed4d5910d1a16601ec7ea0. Authenticated HTTPS ls-remote: `439b13c771ee35cd24a264eb6caf70eea9928eb5 refs/heads/work/r27-approval-deps`.
VERDICT: ACCEPT source change, subject to separate integration/registration and hosted chain qualification; no merge or approval performed.

Complete develop...head diff adds only a three-line step to receipt-approval.yml after existing setup-python: `python -m pip install 'PyYAML==6.0.2'`; same exact dependency pin and command appear in artifact-gc.yml:32. Actual failed approval run 40915351645 job 123594000584 log at 16:49:46 says `receipt blocked: Planning schema resolver requires PyYAML`. Approval's approve_pr -> task_binding -> schema resolver imports yaml lazily (receipt_verifier.py:573); install addresses that observed failure. Inspecting call paths: shadow runs `receipt_verifier.py retained`, which calls verify_file/load_plan/history and not task_binding/schema resolver; post-run executes receipt_tasklink and writer/live, not this YAML resolver. No direct yaml import identified on those paths. This is static path analysis, not proof that a clean hosted runtime has no other missing dependency. Do not add unnecessary steps on these workflows without a demonstrated requirement.

Local detached candidate worktree: `python3 -m unittest test_receipt_postrun test_receipt_chain test_receipt_writer test_receipt_tasklink` exit 0, Ran 73 tests in 5.485s, OK (ResourceWarning); PyYAML safe_load parses approval, post-run and shadow YAML. actionlint unavailable locally; hosted ci-actionlint passed. Worktree clean. GitHub Actions run 40916679898 has exact-head PR Selected Checks success from app 15368; selected validation/focused jobs pass, others skip and are not asserted green.

Pin consequence: receipt-approval.yml checkout uses authenticated run head, full history (:64-69), not an independent develop blob; :94 obtains TRUSTED_SOURCE, and receipt_verifier.py:338-350 requires both verifier bytes and approval workflow bytes at full-SHA pin equal the executing run's corresponding bytes. This PR changes compared workflow bytes. An authorized later registration must set a reviewed full SHA whose workflow/verifier bytes match the final integrated authenticated main run head; pointing at prior 115e4a73 after this workflow is registered would fail closed. A develop-only merge does not change the default-branch workflow; do not claim hosted fix or change a variable here. Retest actual pinned-source and hosted approval after separately authorized integration/registration, and stop on mismatch or unavailable dependency. No secrets inspected, workflow dispatched, push, merge or approval performed. Turn-start observations inspected; Codeman visibility unknown, unrelated shared state untouched.

END rv1209 DONE
