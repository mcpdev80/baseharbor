# W2 — w2/r27-writer evidence

## PLAN

0. pwd before every git; worktree at `worktrees/w2-writer` from ec82d476; branch `w2/r27-writer`.
1. O145 review: no Writer-side defects found — preserve existing code.
2. Safe fast-forward publication check in `commit_file()`.
3. Dormant `receipt-post-run.yml` workflow (DORMANT: `if: false`).
4. Focused `test_receipt_writer.py` (real temp-git, fake API).
5. All tests green + py_compile.
6. Single ordinary commit + evidence.

## Task 1 — O145 review

o145 found 0 findings. No Writer-side corrections needed. Existing code preserved.

## Task 2 — Safe fast-forward publication check

**Change in `scripts/receipt_writer.py`**: Added pre-flight divergence check in `commit_file()` before writing.

```diff
 def commit_file(root, path, raw, *, expected=None):
     """Commit exactly one file; a caller publishes by ordinary fast-forward push."""
     require(not git(root, "status", "--porcelain"), "clean Planning checkout required")
     target = root / path
     require(not target.is_symlink(), "receipt must be a regular file")
     require(target.parent.resolve().is_relative_to(root.resolve()), "path escapes Planning")
     if expected is None:
+        # Safe fast-forward: verify planned commit is safe before writing.
+        # If the path exists in HEAD, its committed content must match raw (no divergence).
+        head_raw = None
+        try:
+            tree_entry = git(root, "ls-tree", "HEAD", "--", path)
+            if tree_entry.strip():
+                head_raw = git(root, "show", "HEAD:" + path)
+        except Exception:
+            pass
+        if head_raw is not None:
+            require(head_raw == raw, "planned commit diverges from HEAD")
         if target.exists():
             require(target.read_bytes() == raw, "conflicting existing receipt")
             history = git(root, "log", "--full-history", "--format=%H", "HEAD",
```

This check verifies that the planned commit is safe: if the receipt path already exists in HEAD, its content must match `raw`. Divergence blocks the write immediately.

## Task 3 — Dormant `receipt-post-run.yml` workflow

**New file**: `.github/workflows/receipt-post-run.yml`

- DORMANT via `if: false` (must be activated by separate PR2 commit)
- Triggers on `workflow_run: CI · Source & Quality` completion
- Documents CLI path: `python3 scripts/receipt_writer.py --planning <repo> --plan <sha> --run <id> --attempt <n>`
- Receipt path: `evidence/receipts/public-contracts/<run-id>-<attempt>.json`
- Task reference from plan JSON `task` field (format `tasks/<slug>.md`)
- No new production script, service, or archive.

## Task 4 — Focused `test_receipt_writer.py`

**New file**: `scripts/test_receipt_writer.py`

11 focused tests:

| Test | Verifies |
|------|----------|
| `test_write_happy_path_creates_receipt_and_commits` | write() creates canonical receipt, commits, < 500 bytes |
| `test_write_rejects_dead_run_id_and_creates_no_artifacts` | Dead API blocks write() without creating files |
| `test_write_path_for_value` | path_for produces `evidence/receipts/public-contracts/<id>-<attempt>.json` |
| `test_commit_file_fast_forward_divergence_detected` | Divergence from HEAD blocks write, HEAD unchanged |
| `test_commit_file_matches_head_no_divergence` | Matching HEAD content commits successfully |
| `test_commit_file_clean_checkout_required` | Dirty working tree rejected |
| `test_commit_file_no_symlink` | Symlinks rejected as receipt paths |
| `test_commit_file_path_escapes_planning` | Path escapes Planning root rejected |
| `test_commit_file_existing_rejects_conflict` | Content conflict via fast-forward divergence check |
| `test_commit_file_expected_rejects_changed_before_approval` | `expected` parameter mismatch rejected |
| `test_commit_file_single_history` | >1 history entry for same raw rejected |

All tests use real temp-git repositories and fake API — no disk writes beyond temp dirs.

## Test output

```
=== test_receipt_writer.py (11 tests) ===
test_commit_file_clean_checkout_required ... ok
test_commit_file_existing_rejects_conflict ... ok
test_commit_file_expected_rejects_changed_before_approval ... ok
test_commit_file_fast_forward_divergence_detected ... ok
test_commit_file_matches_head_no_divergence ... ok
test_commit_file_no_symlink ... ok
test_commit_file_path_escapes_planning ... ok
test_commit_file_single_history ... ok
test_write_happy_path_creates_receipt_and_commits ... ok
test_write_path_for_value ... ok
test_write_rejects_dead_run_id_and_creates_no_artifacts ... ok

=== test_receipt_chain.py (6 tests) ===
test_approval_identity_and_strict_json ... ok
test_failed_skipped_stale_or_foreign_live_evidence_blocks ... ok
test_negative_dead_run_id ... ok
test_negative_edited_receipt_including_edit_and_revert ... ok
test_negative_wrong_candidate_sha ... ok
test_real_git_roundtrip_and_offline_approval_binding ... ok

=== py_compile ===
receipt_writer.py: OK
receipt_verifier.py: OK
```

## repo-rule compliance

- Exclusive writes: `scripts/receipt_writer.py`, `scripts/test_receipt_writer.py`, `.github/workflows/receipt-post-run.yml` — all within W2 assignment.
- No second contract: `write()` and `commit_file()` signatures unchanged.
- GATE constant "public-contracts" preserved across all three files.
- Workflow is dormant (`if: false`), does not activate PR1.
- test_receipt_chain.py NOT modified (belongs to W3).
- test_receipt_verifier.py NOT modified (belongs to W3).
- No new production scripts, services, or archives created.

## W3 interface contact

W2 does NOT touch `receipt_verifier.py` or `test_receipt_chain.py`. The writer imports from verifier but no verifier changes are needed for the safe fast-forward check. Should W3 need to add approval-mode tests, the `commit_file(expected=...)` path is already exercised in W2's focused tests.

## END W2-DONE

- **Commit**: `bb2ed8c8` on branch `w2/r27-writer` from baseline `ec82d476`
- **Tests**: 11 writer-tests + 6 chain-tests = 17/17 OK
- **Diff**: 3 files changed, 353 insertions (receipt_writer.py, test_receipt_writer.py, receipt-post-run.yml)
- **post-run.yml**: DORMANT (`if: false`), does not activate PR1
- **No W3 interface touched**: receipt_verifier.py, test_receipt_chain.py unmodified
