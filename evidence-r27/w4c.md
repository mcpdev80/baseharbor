BOOT: `git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor worktree add ../worktrees/w4c -b work/r27-fix-f6-outputs2 origin/work/r27-correction-v35` -> "Preparing worktree (new branch 'work/r27-fix-f6-outputs2')" / "branch 'work/r27-fix-f6-outputs2' set up to track 'origin/work/r27-correction-v35'." / "HEAD is now at f57b30ed Merge commit '1d8237e9' into HEAD" (pwd=/home/marcel/dev/baseharbor-ecosystem)

## PLAN

1. [x] Boot: worktree `work/r27-fix-f6-outputs2` from `origin/work/r27-correction-v35` (f57b30ed).
2. [x] Reality check: CODEX-REVIEW-f57b30ed48f1.md F6 (:118-128), F4 verifier side (:90-100), F5 verifier side (:483-491), recipe/test (:124-128); reproduced the 3 origin-less approval errors locally (baseline: 67 tests, 3 errors).
3. [ ] `scripts/receipt_verifier.py`: CLI emits exact `plan`/`candidate` (no longer introduction/approval); `publish_approval` returns the final durable card-publication SHA; evidence = immutable public reference validated at the card revision; `task_binding` uses one Planning-schema-aware front-matter resolver plus the injected forward tasklink resolver (API always forwarded, never real in tests); `main()` fails closed on missing/invalid outputs.
4. [ ] `.github/workflows/receipt-approval.yml`: `publication` carried through step/job outputs into the reusable shadow call; step exits nonzero explicitly when any required output line is missing; reusable call pinned to a reviewed immutable full SHA (no mutable `@develop`).
5. [ ] `.github/workflows/receipt-shadow.yml`: consumes the exact final publication (`retained --revision` = publication, never latest Planning HEAD); no `if:` skip on missing inputs — explicit fail; publication full-SHA validation.
6. [ ] Tests (`scripts/test_receipt_verifier.py`): deliberately distinct introduction/approval/plan/candidate/publication values through the CLI plumbing requiring the exact tuple; missing-output and publication-fail paths; shadow shell executes retained verification against the publication even after Planning HEAD moved; real temporary bare Planning remote so the three approval regressions pass meaningfully; PR-only human input preserved; no retained API fallback.
7. [ ] actionlint + receipt verifier suite + `scripts/ci-workflow-policy.sh` green locally.
8. [ ] Push branch, DRAFT PR base `work/r27-correction-v35`, evidence with command+result per point.

## Findings

- Conflict note: `baseharbor-plan/AGENTS.md` records a "v0.4.27 CI pause" (2026-10-27); the Owner's direct lane dispatch (this prompt) explicitly orders this correction work, so the lane executes per Owner instruction. Nothing is accepted or merged by this lane; no card-status mutation.
- `work/r27-fix-f1b-recovery` (w4a) does not exist on origin yet. At the reviewed head, `receipt_tasklink.publish_evidence()` returns the durable card SHA (str) and embeds its third positional argument into the evidence line. The verifier therefore passes the immutable public reference at that position; w4a's exclusive files are untouched.
- The reusable-workflow pin cannot be a literal self-SHA of this branch; following the established pattern (`receipt-post-run.yml:216` controller pin, `TRUSTED_SOURCE` note in `receipt-approval.yml:26-29`), the pin is a reviewed immutable full SHA and its final-integration update remains a coordinator prerequisite, exactly as the review states ("Registration pin ... remain prerequisites, not verified readiness").
