ROLLE: REVIEW

## PLAN
- [x] 0. Read dispatch and policies; record plan. Native TodoWrite unavailable; no native update claimed.
- [ ] 1. Refresh candidate and compare trees, parents and main/develop defenses.
- [ ] 2. Run focused tests, verify exact-head CI and evaluate promotion after the specified merge method.

UTC: 2026-10-29T14:25:09Z
Reviewer Codex GPT-6, independent of worker; no Horde ID exposed. Head 4c20d676dfdc82f597434a5e64bc7d95ddf409bc, develop f95b3446ed9a007a31fcc692396d6dfaf3b38c36, main 7a417d378292724e1820c976b110fe3c107eba45. Authenticated HTTPS ls-remote: `4c20d676dfdc82f597434a5e64bc7d95ddf409bc refs/heads/work/r27-develop-main-reconcile`.
VERDICT: REQUEST_CHANGES / integration acceptance not yet demonstrated.

Candidate tree and accepted 089572fc tree both da634e7412404c4254feb357d79f2505e1dab22a. Candidate parents are 089572fc and current main. For post-run workflow and writer, direct diff main->candidate adds only the write-token mapping/comments; writer has no diff, so inspected main defenses are retained. Develop approval variable and post-run token mapping remain in that tree. Focused quartet on detached candidate exited 0, Ran 73 tests in 5.656s, OK (ResourceWarning); clean worktree. Exact-head run 40890772367: PR Selected Checks success from Actions app 15368, relevant focused/actionlint/policy checks pass and unrelated jobs skip.

The task also requires clean merge-tree main->develop and #1205 promotable. `git merge-tree --write-tree origin/main 4c20d676` succeeds because main is a direct parent of this candidate; it is not a test of the result of the required squash merge to develop. A squash commit has only the former develop parent and does not preserve candidate's main-parent ancestry. Current local `merge-base --is-ancestor origin/main origin/develop` exits 1 in this shallow repository; absence of ancestry is not a complete-history proof but demonstrates the successful candidate merge-tree cannot simply be transferred to a future squash. #1205 cannot yet be declared conflict-free after landing based on this evidence. GitHub rules/standing instructions require ordinary squash and do not authorize switching merge strategy to retain parent.

Recipe: coordinator/worker should produce an isolated complete-history simulation with the actual intended squash commit/tree and parent on current develop, then merge that simulated result into current main and inspect conflict stages and all main-side defenses, or supply an authoritative task-contract decision on a different acceptance sequence. Re-run on the final live bases. Stop if the simulated merge is conflicted, removes defenses, or required exact-candidate CI is not green. No source modification, merge, approval or push performed; no assertion that #1205 is currently promotable. Turn-start events/reminders inspected; Codeman visibility unknown and shared unrelated state left unchanged.

END rv1206D DONE
