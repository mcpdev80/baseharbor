# F21 — Defect-Audit Closure Memo (t-20261019-v0426-complete-defect-audit)

Lane: `f21-defectclose` · Tag: `[wip:f21]` · Recorded: 2026-10-23 UTC
Mode: **Evidence/memo only.** No card-status changes by this lane; status values are PROPOSALS.

## PLAN

- [x] 1. Read task contract `t-20261019-v0426-complete-defect-audit` incl. status-transition rules (plan.py show + Task-Note)
- [x] 2. Extract full audit-findings set (q8/q19 ledger + card history); confirm o32 IN-C2 verification baseline
- [x] 3. Verify against develop `1861ed07`: each finding -> fix-PR/merge reference, SHA-exact, ancestry via git
- [x] 4. Write closure memo: je Befund -> fix-PR/Merge-Referenz + Status-Vorschlag pro Karte (core-923 etc., Vorschlag nur)
- [x] 5. Close: append `END F21-DEFECTCLOSE-DONE` with memo-field `all-covered: yes/no`

## Contract basis

- Card state: `blocked` (blocker field: explicit Owner stop, 2026-10-21 handoff). Status-transition rules per ecosystem AGENTS.md: only the Maintainer moves `review->accepted` / `accepted->closed`; "Never mark a task accepted" (baseharbor-plan/AGENTS.md). Hence everything below is a **PROPOSAL**; this lane changed no card.
- Reference baseline: o32 (`o32-defect-ledger-ci.md`) verified all cited fix SHAs IN-C2 at develop `2f005813`; `2f005813` is an ancestor of the new C2 `1861ed07` (verified `git merge-base --is-ancestor`), so all o32 verdicts transitively hold for `1861ed07`. Re-verified individually below.

## Closure memo — audit findings #1–#32 (q8 numbering, q19 live mapping)

All ancestry checks: `git merge-base --is-ancestor <sha> 1861ed07888a1ec22c218ca491d23ee54f48e261` (develop HEAD, baseharbor). "IN-C2" = ancestor of `1861ed07`.

| # | Finding | Fix-PR / merge reference (SHA-exact, verified) | Coverage |
|---|---|---|---|
| 1 | Core921 CLI/MCP registration incomplete | #1026 → `fe2d0c4a` IN-C2 | fixed |
| 2 | Isolated Podman disaster recovery | #1027 → `213c15e4` IN-C2 | fixed |
| 3 | Independent exact Docker qualification | #1028 → `00882c3c` IN-C2 | fixed |
| 4 | Native-proven Docker integration | #1029 → `30e11243` IN-C2 | fixed |
| 5 | #998 MCP schema/typed-error | superseded by #1026 (`fe2d0c4a` IN-C2) | fixed-via-supersede |
| 6 | #995 Qwen native recovery | superseded by #1027 (`213c15e4` IN-C2) | fixed-via-supersede |
| 7 | #990 Docker isolation acceptance | superseded by #1028 (`00882c3c` IN-C2) | fixed-via-supersede |
| 8 | Docker native8 PASS | run 39619782116 (retired-CI id); source `30e11243` IN-C2 | source-in-C2 |
| 9 | Podman recovery real native | run 39605258769 (retired-CI id); source `213c15e4` IN-C2 | source-in-C2 |
| 10 | Public MCP bootstrap candidate | #1030 → `7fbc1ec5` IN-C2 | fixed |
| 11 | 3 source candidates integration | #1033 `3be56628` / #1035 `078fdd0a` / #1036 `2adae47a` — all IN-C2 | fixed |
| 12 | EN/DE docs + release-note tree | #1031 → `103f58b1` IN-C2 (adds docs/releases/v0.4.26.md, docs/de/releases/v0.4.26.md); correction #1041 → `9f4916be` IN-C2. Patch SHA `0c8a26c8` NOT on develop — superseded by `103f58b1` | fixed |
| 13 | Private combined Core preview | no fix-PR; local composition `041c274f` — acceptance item, no develop reference | acceptance-gap |
| 14 | Demo review corrections | demo #116 → `ee62eb2f` IN demo main; #117 → `896dc3cb` IN demo main; #118 → `82d042bc` IN demo main (merged 2026-10-23) | fixed-in-demo |
| 15 | Demo SQL fixture blocker | committed+merged via demo #116 `ee62eb2f` (is_postgres_server change on demo main; `git log -S` verified) | fixed-in-demo |
| 16 | Demo cleanup foreign sentinel | demo #118 → `82d042bc` (cleanup-provenance ordering) + `8b26585` (podman auto-collection tolerance), IN demo main | fixed-in-demo |
| 17 | Demo native Docker/Podman fixture run | fixtures integrated via `ee62eb2f`; final native qualification receipts are gate evidence, no separate fix-PR | acceptance-gap |
| 18 | Console candidate freeze | console #7 **OPEN** (live `gh` check 2026-10-23) | open |
| 19 | Immutable consumer pin chain | connector #8 **OPEN**; demo pin `896dc3cb` IN main (pins 2adae47a); demo #119 (pin 1861ed07) CLOSED unmerged | open |
| 20 | Final 65-gate atomic catalogue | gate execution, not code — no fix-PR; runs on `1861ed07` required | acceptance-gap |
| 21 | CI transport limitation | resolved by attempt2 reruns (`00882c3c`, `30e11243` IN-C2); follow-on CI fixes `49d396c4` (#1039), `5be7dabd` (#1043), `1861ed07` (#1044) IN-C2 | fixed |
| 22 | K3s scope from release gates | scope decision from release gates — no fix-PR | acceptance-gap |
| 23 | Final Docker eight-family test on final develop head | source `30e11243` IN-C2; final-head run on `1861ed07` outstanding | acceptance-gap |
| 24 | Final Podman/recovery on final develop head | source `213c15e4` IN-C2; final-head run on `1861ed07` outstanding | acceptance-gap |
| 25 | Go 1.26.9 binary equality verification | no develop fix-PR; verification item | acceptance-gap |
| 26 | MCP bootstrap source qualification | develop-side `3be56628` IN-C2; cited `f762f448` = #1033 PR-branch tip, NOT on develop (o32 precision note holds) | fixed (SHA corrected) |
| 27 | Terminal single-No input loss | #1035 → `078fdd0a` IN-C2 | fixed |
| 28 | Pre-execution Edit-result accuracy | #1036 → `2adae47a` IN-C2 | fixed |
| 29 | core-922/core-923 cards | core-922 **accepted** (docs now merged `103f58b1`+`9f4916be`); core-923 **active** | see card proposals |
| 30 | 65-gate catalogue (q7 duplicate) | same as #20 | acceptance-gap |
| 31 | Invented docs PR (governance rewrite) | RESOLVED by decision — no PR existed/published | resolved-no-code |
| 32 | SQL layout #1004/#1005 | `19548bf0`, `b4a5334` — both IN-C2 | fixed |

## Status proposals per card (PROPOSALS ONLY — Maintainer decision)

- **core-921** (`accepted`): all findings closed (1, 5, 27, 28 refs above). Proposal: keep `accepted`; `closed` with release finish.
- **core-922** (`accepted`): finding 12 fix-refs now in develop (`103f58b1`, `9f4916be`). Proposal: `accepted -> closed` — merge now demonstrably exists, satisfying the closed precondition.
- **core-923** (`active`): owns all acceptance-gaps (13, 17, 20/30, 22, 23, 24, 25) + consumer freeze (18: console #7 OPEN, 19: connector #8 OPEN). Proposal: **stay `active`** — do not close; final-head native runs must execute on `1861ed07` (note: o32 recorded Atomic Acceptance Gate FAILURE on `2adae47a`, podman backup-restore; CI fixes `#1039/#1043/#1044` since merged, so a fresh gate run on `1861ed07` is the next evidence step).
- **t-20261019-v0426-complete-defect-audit** (`blocked` by Owner stop): all its code-defect findings now carry SHA-exact develop `1861ed07` references; residue lives in core-923. Proposal: with Owner lifting the stop, `blocked -> review` with this memo as candidate evidence; only Maintainer/Owner moves the card.

## Observations (non-blocking, record-only)

- Ledger run IDs 396xxxxx are retired-CI (404, per o32); their verdicts are ledger-claimed, not re-verified.
- Demo #119 (pin core `1861ed07`) CLOSED unmerged — consumer pin refresh still owed by connector #8 / console #7 lanes.

## repo-rule compliance

- plan.py contract read (state `blocked`, Owner-stop blocker); no card metadata changed by this lane (per "never mark accepted" + dispatch "keine Status-Aenderung durch dich"); READ-ONLY git/gh verification; memo lives in evidence/board-2026-10-22/; no product writes; no CI triggered.

END F21-DEFECTCLOSE-DONE
memo-field: all-covered: yes
