ROLLE: REVIEW

## PLAN
- [x] 0. Read dispatch and policies; record plan. Native TodoWrite unavailable; no native update claimed.
- [ ] 1. Refresh exact candidate and inspect merge ancestry, conflict resolution and both sides.
- [ ] 2. Run focused suites and inspect exact-head CI; record verdict and recipe.

UTC: 2026-10-29T14:13:54Z
Reviewer Codex GPT-6, independent of worker; no Horde task ID exposed. Head 8927676294405bafd5eeb826e5a87481c435d9db; parents f95b3446ed9a007a31fcc692396d6dfaf3b38c36 (develop) and 7a417d378292724e1820c976b110fe3c107eba45 (main). Authenticated HTTPS ls-remote: `8927676294405bafd5eeb826e5a87481c435d9db refs/heads/work/r27-develop-main-reconcile`.
VERDICT: REQUEST_CHANGES.

The supposedly reconciled merge commit has exactly the develop tree: both tree hashes are 2a17260d7096faee1cc7b08c349cb8adbbd4dd18; `git diff origin/develop...89276762` is empty. `git merge-tree --write-tree origin/main 89276762` exits 0 with that tree, but main is a parent of the candidate, so this trivially clean ancestry check does not establish preservation of main content. The resulting direct diff from main removes the same critical hardening identified in RV1205: post-run GH_WORKFLOW_TOKEN, PR-number fallback and authenticated attempt forwarding; writer --product support is removed. The candidate versus previously reviewed 089572fc removes those changes from three files. Thus no true union and no proof of promotion without deleting main defenses. Approval workflow retains the variable-based guard, write-token mapping and full-history checkout; that does not compensate for post-run/writer regressions. contracts/contracts_test.go carries develop's vector addition; test_receipt_postrun.py reverts main-specific test changes. No claim that all requested approval features are present is made.

Required CI is red at exact head: run 40888772895, PR Selected Checks failure from app 15368. Job log: `BLOCKED: coverage-unresolved: empty or unverified changes cannot exempt checks` and `BLOCKED: no required gates or validations`. The prompt's green-CI assertion is contradicted by live evidence. Other executed jobs pass and others skip; no skip is called green.

Worker recipe: reconstruct a genuinely reviewed union from main and the previously accepted 089572fc changes, retaining main's post-run identity/attempt/token hardening and writer --product plus develop's variable pin/write-token and tests; compare actual resulting tree and file diffs against BOTH parent trees, not only a merge-tree call where main is already an ancestor. Verify main defense lines individually and resolve any conflicts with appropriate tests. Obtain a real exact-candidate selected-check success (without bypass or synthetic no-op) and independent review. Stop if either main defense disappears, proposed integration is unverified, or required CI remains red.

Detached candidate worktree quartet `python3 -m unittest test_receipt_postrun test_receipt_chain test_receipt_writer test_receipt_tasklink`: exit 0, Ran 73 tests in 5.849s, OK, ResourceWarning. Passing tests do not establish union. Worktree clean, no merge, approval, push or product edits. Turn-start events/reminders inspected; Codeman visibility unknown, unrelated shared state not modified.

END rv1206C DONE
