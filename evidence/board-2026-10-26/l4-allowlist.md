# L4 Allowlist Evidence — ci-workflow-policy.sh (receipt post-run + approval)

Mission: `ops/missions/l4.txt`. Lane branch `l4/ustrack` from `084b3abc` (integrate: L3 policy
expectations). Worktree `/home/marcel/dev/baseharbor-ecosystem/worktrees/l4-ustrack`. KEIN push.

## STATUS: L4-DONE

Both BELEG gates are green at HEAD `cb01d090` (no push):

| Gate | Result |
|---|---|
| `bash scripts/ci-workflow-policy.sh` (real CI gate, `ci.yml:39/79/723`, `pre-release.yml:87`) | **PASS — 22 approved workflow files, exit 0** |
| `python3 scripts/test_ci_workflow_policy_receipts.py` | **OK — 5 passed, 0 Violations, exit 0** |
| `bash -n scripts/ci-workflow-policy.sh` | OK |
| chain / writer / verifier tests | 6 / 11 / 21 passed, exit 0 (unauffaellig) |

Commits (two ordinary commits, no amend, exactly one file each):
- `29115985` "ci(policy): allowlist receipt post-run and approval workflows" — `scripts/ci-workflow-policy.sh` (+2)
- `cb01d090` "ci(policy): sync expected allowlist with registered receipt workflows" — `scripts/test_ci_workflow_policy_receipts.py` (+42/-38)

## PLAN

- [x] Step 0 — PLAN written to TodoWrite + this evidence file (turn-start basics-check ran)
- [x] pwd-PFLICHT; create worktree `l4/ustrack` @ `084b3abc`; confirm HEAD SHA
- [x] Reality-check the FAIL: `bash scripts/ci-workflow-policy.sh` -> diff `+receipt-approval.yml` / `+receipt-post-run.yml`, exit 1
- [x] Edit file #1 `scripts/ci-workflow-policy.sh`: add both entries (plain, alphabetical, shadow-line style) -> commit `29115985`
- [x] BELEG at commit #1: real gate PASS; bash -n OK; chain/writer/verifier PASS; **receipts test still exit 1 (out-of-scope)**
- [x] Escalate internal contradiction (line 2 "exakt 1 Datei" vs line 3 "test -> exit 0") -> **Coordinator decision: Option 1, write-set widened by exactly one file**
- [x] Edit file #2 `scripts/test_ci_workflow_policy_receipts.py` -> commit `cb01d090`
- [x] BELEG final: receipts test -> 0 Violations + exit 0 (all green)
- [x] Update this evidence (decision + testausgabe); verdict END L4-DONE

## Diff — commit 1: `scripts/ci-workflow-policy.sh`

```diff
@@ -15,6 +15,8 @@ expected=(
   podman-acceptance.yml
   pre-release-runtime-suite.yml
   pre-release.yml
+  receipt-approval.yml
+  receipt-post-run.yml
   receipt-shadow.yml
   release.yml
   renovate.yml
```

## Diff — commit 2: `scripts/test_ci_workflow_policy_receipts.py` (sync to registered state)

- `EXPECTED_ALLOWLIST`: added `"receipt-approval.yml"`, `"receipt-post-run.yml"` (alphabetical before `receipt-shadow.yml`).
- Module + class docstrings and `RECEIPT_FILES` metadata: rewrote the stale "FAILS on purpose until …" /
  workflow-level expectation text to the landed state (L1/L2 job-level guards + allowlist registered => green).
- Two obsolete negative tests were reconciled to the registered truth (see note below):
  `test_receipt_post_run_not_in_allowlist` -> `test_receipt_post_run_in_allowlist_with_job_level_guard`,
  `test_receipt_approval_not_in_allowlist` -> `test_receipt_approval_in_allowlist_with_job_level_guard`;
  assertions flipped `assertFalse(in_allowlist)` -> `assertTrue` and `assertIn("workflow-level", …)` ->
  `assertIn("job-level", …)`, mirroring the already-passing `test_receipt_shadow_in_allowlist_with_job_level_guard`.

## Testausgabe

### `bash scripts/ci-workflow-policy.sh`

```
BEFORE (084b3abc): GitHub Actions workflow policy violation. ... +receipt-approval.yml +receipt-post-run.yml ... exit=1
AFTER  (HEAD):     Workflow policy PASS: 22 approved workflow files.   exit=0
```

### `python3 scripts/test_ci_workflow_policy_receipts.py`

BEFORE commit 2 (only commit 1 applied) — still exit 1:
```
.FF.F   Ran 5 tests ... FAILED (failures=3)
  [VIOLATION] 'receipt-approval.yml' is NOT in the allowlist
  [VIOLATION] 'receipt-post-run.yml' is NOT in the allowlist
exit=1
```

AFTER commit 2 (`cb01d090`) — goal met:
```
.....
----------------------------------------------------------------------
Ran 5 tests in 0.001s

OK
exit=0            # 0 VIOLATION lines (grep -c VIOLATION == 0)
```

### chain / writer / verifier (unchanged by the edit)

```
test_receipt_chain.py     6 passed, 9 subtests passed   exit=0
test_receipt_writer.py    11 passed                     exit=0
test_receipt_verifier.py  21 passed                     exit=0
```

## Coordinator decision & reconciliation note

The original mission was internally contradictory: line 2 restricted the write-set to `ci-workflow-policy.sh`
("KEINE weitere Datei aendern"), but line 3's BELEG (`test_ci_workflow_policy_receipts.py` -> exit 0, 0
Violations) is not reachable through that file alone — the test never reads `ci-workflow-policy.sh`; it holds
its own `EXPECTED_ALLOWLIST` mirror (line 33) and asserts a `workflow-level` dormancy form that L1/L2
(commits `9b185fd2`, `818f4889`, already in base `084b3abc`) had already replaced with job-level guards
(`receipt-post-run.yml:24`, `receipt-approval.yml:56`). Escalated; Coordinator chose **Option 1** and widened
the write-set by exactly one second file, `scripts/test_ci_workflow_policy_receipts.py`.

Execution note (transparency beyond the literal sub-instruction): the Coordinator named the
`EXPECTED_ALLOWLIST` + docstring edits. Those two alone do NOT reach the stated green goal — after adding the
two names, the two `*_not_in_allowlist` methods flip to failing (`assertFalse(True)`) and their
`assertIn("workflow-level", …)` still fails against the landed job-level guards. To honour the binding stop
criterion "Test muss exit 0 mit 0 Violations" while staying inside the single authorized file, those two
now-obsolete negative assertions (and their `RECEIPT_FILES`/comment metadata) were reconciled to the registered
state. No YAML, no other file, and no CI config was touched. Flag for reviewer: if strictly the two enumerated
edits were intended, the green goal would be unattainable — this reconciliation is why it is green.

## Handoff

- Branch `l4/ustrack`, HEAD `cb01d090` (parents `29115985` -> `084b3abc`). Nothing pushed.
- Real `Workflow policy` gate PASS; receipts regression test green (0 violations); chain/writer/verifier green.
- Awaiting the normal independent review + repo-mandated CI before merge; planning status update per lifecycle step 11.

END L4-DONE
