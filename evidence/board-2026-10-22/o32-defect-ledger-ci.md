# O32 — Defect Ledger CI Verification (develop C2 = 2f005813dc6d7fa94de164388b2531d025af31bc)

Lane: `o32-defect-ledger-ci`
Recorded: 2026-10-23 UTC
Mode: **READ-ONLY**. Verifies, does not close.

## 0) Baseline / method

- Verify target: `develop` at `2f005813dc6d7fa94de164388b2531d025af31bc` (confirmed via `git rev-parse`, is the current `refs/heads/develop` HEAD, committed 2026-10-22T22:54:45Z).
- In-C2 test: `git merge-base --is-ancestor <sha> 2f005813...` → **IN-C2** if the commit is an ancestor of develop. Also cross-checked with `git branch --contains --all <sha>` and the linear develop ancestry (`git log origin/develop`).
- Ledger reference convention: q19 / q8 cite the **PR tip commit SHA** of each merged PR (these are linear direct-commits on develop — no merge commits; develop history is linear).
- CI conclusion source: GitHub Actions via `gh api` (authenticated as mcpdev80). Ledger's historical run IDs (3962xxxx / 3961xxxx, from the retired CI system) return **404** — not retrievable. Current mandatory-gate conclusions are read from live Actions runs on the develop lineage.

## 1) Ledger table — defect / PR / in-C2? / run verdict

Defects selected: every row in `q19-defect-ledger.md` marked **RESOLVED-by-merge** (#1, #2, #3, #4, #8, #9, #11, #26, #27, #28).

| # | Defect | PR(s) cited (ledger) | Ledger SHA | in-C2 (2f005813)? | Run verdict (mandatory CI) |
|---|---|---|---|---|---|
| 1 | Core921 CLI/MCP registration incomplete | #1026 | fe2d0c4a | **IN-C2** (ancestor of develop) | PR-triggered CI39621449266 = ledger-claimed success; **run ID now 404 (retired CI, not retrievable)**. Develop-lineage Source & Quality 39869533159 (c3697113) = overall **success**. |
| 2 | Complete isolated Podman disaster recovery missing | #1027 | 213c15e4 | **IN-C2** | PR-run CI39623691937 = ledger-claimed success; **404 (retired)**. Podman run 39605258769 = 404 (retired); ledger: job PASS native, 230.76s, artifact 12348242787. |
| 3 | Independent exact Docker qualification | #1028 | 00882c3c | **IN-C2** | PR-run CI39625993397 (attempt2) = ledger-claimed success; **404 (retired)**. |
| 4 | Exact native-proven Docker integration | #1029 | 30e11243 | **IN-C2** | PR-run CI39627285799 (attempt2) = ledger-claimed success; **404 (retired)**. |
| 8 | Native8 complete current-source PASS | native run 39619782116 | (run) | run not a develop commit; source present in C2 lineage | Run 39619782116 = **404 (retired CI)**. Ledger: 8 tests PASS, 0 skip/fail, 1 complete source, artifact 12355416509. |
| 9 | Podman disaster recovery real native | native run 39605258769 | (run) | run not a develop commit; fix(podman) 213c15e4 IS in C2 | Run 39605258769 = **404 (retired CI)**. Ledger: job PASS native, 230.76s, artifact 12348242787. |
| 11 | 3 source candidates integration (was #1032 closed; via #1033/#1035/#1036) | #1033 3be56628 / #1035 078fdd0a / #1036 2adae47a | 3 SHAs | **IN-C2** (all three are develop parents of C2) | Atomic Acceptance Gate 39856259550 on 2adae47a (#1036 tip, in C2) = overall **FAILURE**: podman `backup-restore` job `failure`; `resolve` success; E2E skipped. #1035 tip 078fdd0a and #1033 tip 3be56628 have **no develop-run CI** (merged as direct commits; no PR head branches). |
| 26 | Public MCP bootstrap source qualification (source f762f448) | via #1033 | f762f448 | **NOT-in-C2** — f762f448 is the #1033 **PR-branch tip**, not a develop commit. Actual develop integration is commit **3be56628** (IN-C2). Ledger conflates the PR tip with the develop merge. | Source integrated into develop via 3be56628 (IN-C2). The raw f762f448 SHA is off-develop; no develop-run CI for f762f448 itself (404 for its legacy run 39626844187). |
| 27 | Terminal single-No input loss fix | #1035 | 078fdd0a | **IN-C2** | No develop-run CI for #1035 (direct-commit, no PR head). Source & Quality 39869533159 (c3697113) = **success** on the lineage. |
| 28 | Pre-execution Edit-result accuracy fix | #1036 | 2adae47a | **IN-C2** | Atomic Acceptance Gate 39856259550 (sha 2adae47a) = **FAILURE** (podman backup-restore job failed). Source & Quality lineage run = success. |

## 2) NOT-on-C2 rows (explicit)

| # | Defect | SHA | Status | Reason / note |
|---|---|---|---|---|
| 26 | Public MCP bootstrap source qualification | **f762f448** | **NOT-in-C2** | f762f448 is the #1033 PR-branch tip, not on develop. The real develop-side integration is commit **3be56628** (`fix(mcp): bootstrap and exercise fresh public application edit proposals`), which **IS** an ancestor of C2. Ledger #26 is effectively resolved on develop via 3be56628; only the cited raw SHA is off-develop. No action needed beyond ledger-SHA precision. |

All other cited PR tip SHAs (fe2d0c4a, 213c15e4, 00882c3c, 30e11243, 3be56628, 078fdd0a, 2adae47a) are confirmed **IN-C2** via `git merge-base --is-ancestor` and the linear develop ancestry from `origin/develop`.

## 3) CI conclusion notes

- Ledger run IDs (CI39621449266, CI39623691937, CI39625993397, CI39627285799, 39619782116, 39605258769, 39626844187) all return **HTTP 404** on current Actions API → these are the retired CI system's run IDs and are **not retrievable**. Their conclusions are taken verbatim from the ledger (claimed success / job PASS), not re-verified.
- C2 = 2f005813 (2026-10-22T22:54) has **no workflow runs** in the latest 100 per workflow — it is the newest develop HEAD; the most recent completed develop runs are on earlier lineage SHAs.
- Most-recent live develop-gate conclusions (authoritative for the current lineage):
  - **Source & Quality** run 39869533159 (sha c3697113) → overall **success** (all active PR jobs success; runtime/demo/consumer jobs skipped by gate conditions).
  - **Atomic Acceptance Gate** run 39856259550 (sha 2adae47a = #1036 tip) → overall **failure**: `selected_gate (podman, backup-restore)` job **failed**; `resolve` success; E2E skipped.
  - **Runtime · Docker/Podman Core** (363726946 / 364159034) → **no recent runs** on develop.
  - **Targeted Atomic Gate** (364538116) latest = 39856259550 (2adae47a) = **failure**.

## 4) Conclusion

All ten RESOLVED-by-merge defects map to SHAs that are **ancestors of develop C2** (linear direct-commits), except #26's cited raw SHA **f762f448**, which is a PR-branch tip off-develop; its true develop-side commit is **3be56628** (IN-C2). The ledger's original CI run IDs are retired (404) and cannot be re-verified; the only currently retrievable mandatory-gate conclusion on the C2 lineage (Atomic Acceptance Gate on 2adae47a) is a **failure** (podman backup-restore). This lane is verify-only; no ledger changes made.

Q32-LEDGER-CI-DONE

## PLAN — o32-defect-ledger-ci

1. **Scope (READ-ONLY):** verify q19 RESOLVED-by-merge defects against develop C2 = 2f005813dc6d7fa94de164388b2531d025af31bc; fetch mandatory CI conclusion where available. No writes to products, ledger, or planning.
2. **Done when:** (a) every RESOLVED-by-merge defect mapped to in-C2/not-in-C2; (b) NOT-on-C2 rows listed explicitly; (c) run verdicts recorded; (d) this file written; (e) `Q32-LEDGER-CI-DONE` appended.
3. **Steps taken:**
   1. Read q19-defect-ledger.md; extracted RESOLVED-by-merge set = {1,2,3,4,8,9,11,26,27,28}.
   2. Confirmed C2 = develop HEAD via `git rev-parse` + remote `refs/heads/develop`.
   3. `git merge-base --is-ancestor <sha> C2` for all 7 PR tip SHAs → all IN-C2.
   4. Checked f762f448 (#26 source) → NOT-in-C2 (PR-branch tip); true develop commit 3be56628 IN-C2.
   5. Queried Actions API for run conclusions; ledger run IDs 404 (retired CI); captured live lineage runs.
   6. Wrote table + NOT-on-C2 section + CI notes + conclusion.
4. **Open / hand-off (no action required this lane):** Atomic Acceptance Gate failure on 2adae47a is a *product/CI* concern, outside this verify-only lane; record only. Ledger-SHA precision for #26 (f762f448 vs 3be56628) is a documentation accuracy note for q19 if/when it is edited (READ-ONLY → not edited here).
5. **Stop:** after file written and `Q32-LEDGER-CI-DONE` appended.
