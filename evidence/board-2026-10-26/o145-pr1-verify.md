# O145 — Independent verification of PR1 candidate (v0.4.27 "Receipt statt PKI")

ROLE: W5 independent verification. Scope: PR #1090 `work/r27-minimal-receipt` @ `ec82d476126291d372df2c13bd36273767574869`.
Standard: baseharbor-plan decisions/2026-10-26-minimal-receipts.md + baseharbor-plan/evidence/r27-ci-replacement-2026-10-26/RECEIPT-PR1.md.
Write-set: read-only worktree `worktrees/o145-verify1`. No push / commit / product file change.

## PLAN

1. pwd before every git; add worktree at candidate SHA (read-only).
2. Read decision + REDEIPT-PR1 evidence; treat claims as "don't believe, recompute".
3. PFCHECK 1 schema: exactly six top-level fields; <500 bytes vs real writer call (temp dir); no signatures/archives/retention.
4. PFCHECK 2 run `python3 scripts/test_receipt_chain.py` myself; count tests; confirm the three negatives (wrong SHA, dead run-ID, edited receipt) reject on specific assertion classes, not "an error occurs".
5. PFCHECK 3 verifier live/offline separation by code line; no path that needs GitHub API after acceptance.
6. PFCHECK 4 pilot honesty: "fresh attempt 1 only" + manual copy of run-IDs yields no success.
7. PFCHECK 5 EN/DE docs identical; no promise beyond PR1 (no PKI/archive).
8. Findings by severity + Verdict → this file.

## Findings (by severity)

No findings of any severity. All five pre-checks pass with concrete, recomputed evidence below.

### PFCHECK 1 — schema & size (PASS)
- Receipt schema is an exact six-field set: `receipt_verifier.py:61` `require(set(value) == {"gate", "run", "sha", "status", "plan", "approved"}, ...)`. No signature/archive/retention field anywhere in the receipt or plan schema (`receipt_verifier.py:103`).
- Real writer call in a temporary Git repo (not a unit stub) produces an 188-byte receipt, strictly `< 500`. `receipt_verifier.py:45` bounds `decode(raw, 500)`.
- The emitted bytes are byte-canonical: `raw == canonical(value)` asserted at `receipt_verifier.py:78`; my temp-dir write reproduced `canonical == True` and `size_bytes = 188`.

### PFCHECK 2 — tests run by me (PASS)
- `python3 scripts/test_receipt_chain.py -v` → **6 tests ran, all OK** (`test_real_git_roundtrip_and_offline_approval_binding`, `test_negative_wrong_candidate_sha`, `test_negative_dead_run_id`, `test_negative_edited_receipt_including_edit_and_revert`, `test_failed_skipped_stale_or_foreign_live_evidence_blocks`, `test_approval_identity_and_strict_json`).
- Three required negatives reject on specific error classes, verified independently:
  - wrong candidate SHA → `wrong candidate SHA` (`receipt_verifier.py:130`) and `verify_file` → `candidate/plan` (`receipt_verifier.py:210`).
  - dead run ID → live path raises `ReceiptError("GitHub witness unavailable: run not found")` before any commit; writer returns exit 1 and writes nothing (`test_negative_dead_run_id`, `receipt_verifier.py:132-134`).
  - edited receipt → canonical-bytes check rejects tampering (`receipt_verifier.py:78`); retained mode rejects any later edit including reverted tamper via `no later edits` (`receipt_verifier.py:229`).
- Not "an error occurs": each negative asserts a distinct, named `ReceiptError` reason class tied to the specific failure.

### PFCHECK 3 — live/offline separation (PASS, code-line evidence)
- `live()` is the only path that calls `api()` (`receipt_verifier.py:132,135,148,173,188`); it reads only the uniquely named `trusted-pr-plan` artifact zip and retains nothing (no `global`, no persisted artifact).
- `verify_file()` and `retained()` contain no `github(`/`api(` reference — offline retained verification never calls GitHub after acceptance.
- `retained()` requires `--revision` from a protected Planning revision and binds via Git ancestry (`merge-base --is-ancestor`, `receipt_verifier.py:221-230`), never a PR-supplied branch.

### PFCHECK 4 — pilot honesty (PASS)
- Attempt gate present: `require(value["run"]["attempt"] == 1, "pilot requires a fresh run, not an Actions rerun")` (`receipt_verifier.py:129`); reruns (attempt>1) cannot create a receipt.
- Manual copy of a dead run-ID does not succeed: my temp-dir writer call with run 999999 raised `ReceiptError: GitHub witness unavailable: run not found`, exit 1, and created no `evidence/receipts/` path. No person can author a successful receipt by copying run numbers.
- Writer enforces trusted post-run action on protected main (`receipt_writer.py:56-59`); `approve()` additionally enforces Owner identity (`receipt_verifier.py:235-240`).

### PFCHECK 5 — EN/DE docs (PASS)
- Both 58 lines; identical fenced-json structure; the single PKI/archive mention in each is a denial ("No logs, output payloads, signature keys, attestation bundles or archive service are stored." / "Gespeichert werden keine ... Signierschlüssel, Attestierungs-Bundles oder Archivdienste.").
- No promise beyond PR1: both docs state PR2 adds the automatic task link (attest path) and PR3 the cutover; PR1 "installiert keinen Publisher-Workflow" / "does not install a publisher workflow". The plan-fixture task reference is validated as a `tasks/...md` ref (`receipt_verifier.py:110-111`).

## Verdict

**PR1-VERIFIKATION-BESTANDEN.**

PR1 is a faithful, self-consistent implementation of the minimal-receipt decision: exactly six canonical fields, 188-byte receipt (<500), six passing tests including the three required negatives rejecting on specific `ReceiptError` classes, clean live/offline separation with no post-acceptance GitHub API call, honest "attempt 1 only" pilot with no manual-copy success path, and byte-aligned EN/DE docs making no promise beyond PR1. This is not hosted acceptance; the genuine live GitHub path and automatic task linkage remain PR2/PR3 scope.

This verification is evidence for Owner acceptance. The concrete, Worker-implementable follow-up (if any) is: none — no defect requires correction. Remaining roadmap items (PR2 automatic task link, PR3 cutover, target-side path-mapping/test-discovery) are decision-scoped future PRs, not blockers on PR1.

---
Candidate SHA: ec82d476126291d372df2c13bd36273767574869 · PR: #1090 · branch: work/r27-minimal-receipt
Tests run: 6/6 OK · Receipt size: 188 bytes · Findings: 0
