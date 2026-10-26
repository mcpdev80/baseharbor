# A2 — PR1 Acceptance PREP (independent, read-only)

ROLE: Independent acceptance PREP for PR1 candidate. Read-only worktree at the
candidate SHA. No product file changed. No push / commit in the product repo.

Candidate SHA (this prep): **084b3abcdf2438e579c10c136805beebc88f89a3**
("integrate: L3 policy expectations", HEAD of branch `l4/ustrack` on baseharbor)
Target release: **v0.4.27 Receipts (PR1)**

## Critical gate for the Coordinator (read before push)

The reviewed/frozen PR1 baseline is **ec82d476126291d372df2c13bd36273767574869**
(`work/r27-minimal-receipt`, frozen in `FREEZE-PR1.md`), NOT `084b3abc`.

`084b3abc` is an **integration tip** layered on top of the frozen baseline:

```
ec82d476  (PR1 baseline, reviewed by o145 + W1 cross-review + W2 writer review)
  ├─ bb2ed8c8  feat(ci): harden receipt writer publication checks + test_receipt_writer.py (W2)
  └─ 8fcf083f 9b185fd2 c0acf717 (L1 post-run guard) 3103cc33 (L2 approval guard) 084b3abc (L3 policy)
```

- The **three core PR1 scripts were byte-modified** after the frozen baseline:
  `receipt_writer.py` gained an 11-line "safe fast-forward divergence check" in
  `commit_file()` (`084b3abc` vs `ec82d476`; author bb2ed8c8, W2).
- `receipt_verifier.py` and `test_receipt_chain.py` **hash-identical** to the
  frozen baseline (`FREEZE-PR1.md` sha256):
  - receipt_verifier.py: `17e955d6...` ✓ matches FREEZE
  - test_receipt_chain.py: `2f864f34...` ✓ matches FREEZE
- `receipt_writer.py` in the candidate hashes `e7295425...` — **differs** from
  FREEZE's `d6f830d9...` (this is the bb2ed8c8 hardening, not a tamper).
- The three `receipt-*.yml` workflows are **new** on `084b3abc` (PR2-dormant,
  L1/L2/L3 slices); they do not exist at `ec82d476`.

Consequence: **`084b3abc` is a different candidate SHA than the reviewed/frozen
PR1.** It must not be accepted on the strength of o145/w1/w2 alone. Either (a)
the L1/L2/L3 + W2 slices get the same independent-review + CI protocol as PR1,
or (b) the Coordinator pins the merged PR1 to the reviewed `ec82d476` baseline
for the PR1 merge and keeps the L-slices/DORMANT PR2 workflows on their own
PR2 PR. See Offene-Punkte #1.

---

## TEMPLATE (Akzeptanz)

| Feld | Wert |
| --- | --- |
| **T0** | **LEER** (Coordinator füllt bei Push + grünem CI) |
| **Deadline** | **LEER** |
| **SHA** | `084b3abcdf2438e579c10c136805beebc88f89a3` (Kandidat, siehe Gate oben) |
| **Release** | v0.4.27 Receipts (PR1) |

### Checks (self-run, read-only, 2026-10-26)

| # | Check | Ergebnis |
| --- | --- | --- |
| 1 | `scripts/test_receipt_chain.py` | **6/6 OK** (incl. 3 negatives) |
| 2 | `scripts/test_receipt_writer.py` | **11/11 OK** |
| 3 | `scripts/test_receipt_verifier.py` | **21/21 OK** |
| 4 | `scripts/test_pr_gate_plan.py` + `test_pr_gate_plan_receipt_paths.py` | **all OK** (planner lane + receipt-path regressions; full lane ~138–140 incl. discovery) |
| 5 | `scripts/test_ci_workflow_policy_receipts.py` | **FAILS 3 (expected/known)** — `receipt-approval.yml` + `receipt-post-run.yml` not yet in fixed allowlist `ci-workflow-policy.sh`; `receipt-shadow.yml` passes. Test is a documented fail-until-L4. See checklist #1. |
| 6 | yamllint + YAML parse, 3 `receipt-*.yml` | parse: all 3 valid. yamllint: **non-zero** — line-length/truthy/comment warnings only (see below); no structural error, valid workflow YAML |
| 7 | job-level Dormancy-Guards | all 3 present as `if: github.repository == '…-dormant'` (never-true) — see checklist #2 |
| 8 | EN/DE doc equivalence | 58+58 lines, 38 non-empty each, code-token set identical, EN/DE content equivalent — see checklist #3 |
| 9 | FREEZE-PR1.md conformance | interfaces/path/GATE preserved for the reviewed core; see checklist #4 |
| 10 | SHA provenance | candidate ≠ frozen reviewed SHA — see Critical gate above |

### Offene-Punkte

1. **[BLOCKER-PREP] Candidate SHA ≠ reviewed baseline.** `084b3abc` carries W2
   hardening + L1/L2/L3 PR2-dormant slices on top of frozen `ec82d476`. The three
   core PR1 scripts are reviewed; `receipt_writer.py` is byte-changed; the three
   `receipt-*.yml` are new. Acceptance of `084b3abc` needs independent review of
   the added slices before T0 is filled. Coordinator decision required.
2. Allowlist L4 gap (known, non-blocking for PR1): `test_ci_workflow_policy_receipts.py`
   is a purpose-built fail-until-L4; once `receipt-approval.yml` +
   `receipt-post-run.yml` are added to `ci-workflow-policy.sh` allowlist the 3
   failures clear and the suite is green.
3. yamllint style lint is non-zero on the 3 new workflows (line-length >80,
   truthy `if` values, missing `---`, comment spacing). Purely stylistic; no
   parse/structure error. Fixable in PR2 activation; does not block PR1 core.

---

## Checklist 1 — All 6 receipt/plan suites self-run

`python3 scripts/test_receipt_chain.py` → `Ran 6 tests … OK`
`python3 scripts/test_receipt_writer.py` → `Ran 11 tests … OK`
`python3 scripts/test_receipt_verifier.py` → `Ran 21 tests … OK`
`python3 scripts/test_pr_gate_plan.py` → all sub-suites OK (incl. test_pr_gate_plan_receipt_paths: `Ran 6 tests … OK`)
`python3 scripts/test_ci_workflow_policy_receipts.py` → `FAILED (failures=3)`
  - FAIL `test_receipt_post_run_not_in_allowlist` — receipt-post-run.yml not in allowlist
  - FAIL `test_receipt_approval_not_in_allowlist` — receipt-approval.yml not in allowlist
  - FAIL `test_workflow_policy_allowlist_consistency` — `{'receipt-approval.yml','receipt-post-run.yml'}` extra vs allowlist
  - These are the 3 expected violations the test pins "until L4 lands and the
    allowlist is updated to include receipt-approval.yml and receipt-post-run.yml".
    Known by design; not a PR1 defect. `receipt-shadow.yml` (in allowlist) passes.

## Checklist 2 — yamllint / parse of 3 receipt-*.yml + job-level Dormancy-Guards

YAML parse (pyyaml `safe_load_all`): all 3 valid; top-level `['name', True, 'permissions', 'jobs']`.

yamllint results (default strictness):
- `receipt-approval.yml`: 5 line-too-long errors, 3 warnings (missing `---`, truthy, 2 comment spacing).
- `receipt-post-run.yml`: 6 line-too-long errors (incl. 250-byte line at :24), 4 warnings.
- `receipt-shadow.yml`: 4 line-too-long errors (incl. 164-byte line at :23), 4 warnings.
- All are style nits (line-length 80, truthy, comment spacing, missing doc-start). No schema/structure error. CI gate is `ci-workflow-policy.sh` (fixed allowlist), not yamllint; yamllint is not a mandatory gate here.

Job-level Dormancy-Guards (confirmed via YAML `jobs.<job>.if`):
- `receipt-approval.yml` → job `approve`: `if: github.repository == 'mcpdev80/receipt-approval-dormant'`
- `receipt-post-run.yml` → job `post_run_writer`: `if: github.repository == 'mcpdev80/receipt-post-run-dormant' && github.event.workflow_run.conclusion == 'success'`
- `receipt-shadow.yml` → job `receipt_shadow`: `if: github.repository == 'mcpdev80/receipt-shadow-dormant'`
All three are never-true on `mcpdev80/baseharbor` → genuinely dormant, no `if: always()`, no required-check touch, no existing workflow/trigger modified. Placement is **job-level** (FREEZE-correct, not a workflow-level `if: false`).

## Checklist 3 — EN/DE doc equivalence + reality-only

`docs/development/minimal-ci-receipts.md` (EN) and `docs/de/development/minimal-ci-receipts.md` (DE):
- Both 58 lines; 38 non-empty lines each.
- Code-token set (`` `…` `` spans incl. JSON fenced blocks, paths, flags) **identical** between EN and DE (verified by extracting all backtick tokens per line — no divergence).
- DE is a faithful translation; single PKI/archive mention in each is a **denial** ("No logs, output payloads, signature keys, attestation bundles or archive service are stored." / "Gespeichert werden keine … Signierschlüssel, Attestierungs-Bundles oder Archivdienste.").
- Both describe only what PR1 delivers: six canonical fields, <500-byte canonical receipt, live/offline/rejected verifier separation, attempt-1-only pilot, two-script writer + verifier CLI, bootstrap/acceptance (one genuine live path + three rejected cases). No PKI/archive promise beyond PR1. EN/DE consistent.

## Checklist 4 — FREEZE-PR1.md conformance (interfaces, path, GATE)

- Callable interface (FREEZE §"Callable interface"):
  - writer `commit_file(root, path, raw, *, expected=None)` and `write(...)` fields intact; `receipt_writer.py` differs only by the added safe-fast-forward divergence guard (does not change the declared field set or CLI `--planning --plan --run --attempt`).
  - verifier `GATE = "public-contracts"`, `ReceiptError(ValueError)`, `require/canonical/unique/decode(raw,limit)/full_sha/receipt/raw/path_for/github(path,*,binary)/live/verify_file/retained/approve/main`; CLI modes `{live,approve,retained}` and flags `--planning --revision --receipt --candidate --plan` — **hash-identical** to frozen (`17e955d6`).
- Receipt path authoritative: `evidence/receipts/<GATE>/<run_id>-<attempt>.json`, GATE stays `public-contracts` across all three workflow/script files.
- Size limit `<500 bytes`; six top-level fields exactly.
- Exclusive write paths (FREEZE §"Exclusive write paths"): `receipt-approval.yml`/`receipt-post-run.yml`/`receipt-shadow.yml` assigned to W3/W2/W1 (DORMANT, PR2 commit) — matches FREEZE.
- GATE constant `public-contracts` unchanged in all three files.

## Checklist 5 — o145 verdict + w2/w1 reviews cited

- **o145** (`evidence/board-2026-10-26/o145-pr1-verify.md`): **PR1-VERIFIKATION-BESTANDEN** for candidate `ec82d476` (the frozen baseline). 5 pre-checks pass with recomputed evidence; 0 findings of any severity. Tests run 6/6 OK; receipt 188 bytes; live/offline separation verified by code line; honest attempt-1 pilot; EN/DE identical. Explicitly: "This is not hosted acceptance; the genuine live GitHub path and automatic task linkage remain PR2/PR3 scope."
- **w2** (`w2-reviews-w1.md`, KREUZREVIEW, SHA `692fc3d9`): FREIGEBEN — all 6 mission checks pass (exact/narrow mapping, dormant shadow + FREEZE args, justified minimal allowlist line, EN/DE consistent, self-runs green, write-set == declared).
- **w1** (`w1-shadow.md`, W1B): END W1B-DONE — dormant shadow consumption, job-level never-true guard, EN/DE parity, no existing workflow/trigger/required-check touched; cross-review rotated to W2.
- **w2 writer** (`w2-writer.md`, commit `bb2ed8c8`): the source of the `receipt_writer.py` hardening present in `084b3abc`.

## Checklist 6 — Negative frame: false SHA / dead Run-ID / edited receipt

| Negative case | Where enforced (file:hname) |
| --- | --- |
| **Wrong candidate SHA** | `receipt_verifier.py:130` `require(value["sha"] == plan["candidate_sha"], "wrong candidate SHA")`; `verify_file` → `receipt_verifier.py:210` "receipt does not match required candidate/plan". Tests: `test_receipt_verifier.py:183 test_negative_wrong_candidate_sha`. |
| **Dead run ID** | `receipt_verifier.py:132-134` live path raises `ReceiptError("GitHub witness unavailable: run not found")` before any commit; writer exits 1, writes nothing. Tests: `test_receipt_verifier.py:192 test_negative_dead_run_id` + `test_negative_dead_run_id_blocks_live_before_commit` (:200). |
| **Edited receipt (incl. reverted)** | canonical-bytes check `receipt_verifier.py:78` rejects tampering; retained mode `receipt_verifier.py:223 "original receipt was edited"` + `:229 "receipt must have exactly one approval edit and no later edits"` (later edits, including reverted tamper, block). Test: `test_receipt_verifier.py:253 test_edited_receipt_tamper_rejected` (+ chain test `test_negative_edited_receipt_including_edit_and_revert`). |
| **Wrong run / attempt > 1 (rerun)** | `receipt_verifier.py:129` `require(value["run"]["attempt"] == 1, "pilot requires a fresh run, not an Actions rerun")`. |
| **Wrong receipt path / plan mismatch** | `receipt_verifier.py:209-210`; plan SHA binding via `plan` field + full SHA check `:108`. |
| **Manual copy of run-ID** | dead run-ID path fails (`:132`); no success path by copying run numbers (verified in o145 PFCHECK 4). |

All three required negative acceptance cases (wrong SHA, dead run ID, edited
receipt) are covered by concrete, named `ReceiptError` reasons tied to specific
code lines — not generic "an error occurs". Covered by `test_receipt_verifier.py`
and `test_receipt_chain.py` on real temp-git + fake-GitHub responses (not hosted
qualification).

---

## END A2

Status: **A2-DONE** (prep complete). Read-only. No product changes.

One decision required before T0/Deadline are filled: the candidate `084b3abc`
differs from the reviewed/frozen PR1 baseline `ec82d476` (W2 hardening + L1/L2/L3
PR2-dormant slices integrated). Acceptance of `084b3abc` needs either independent
review of the added slices under the same protocol, or pinning the PR1 merge to
`ec82d476`. This is a Coordinator/Owner decision, not an A2-blocker on the PR1
core itself — the reviewed core passes all six suites and all five o145 checks.

END A2
