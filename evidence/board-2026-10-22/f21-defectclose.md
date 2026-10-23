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

## AUDIT

ROLLE: REVIEW — Codex, 2026-10-23. Scope: independent three-finding
spot check requested by `prompts/rw-audit.txt`; no card closure or product changes.

### PLAN

- [x] 0. Read the dispatched prompt and binding policies; establish this review plan before audit work.
- [x] 1. Refresh remote product/planning state and pin the memo and target commit.
- [x] 2. Check three findings against actual GitHub merge SHAs, ancestry and source changes.
- [x] 3. Record the bounded verdict, any actionable correction recipe and remaining acceptance limits.

Tool limitation: this Codex session exposes no native TodoWrite/todowrite tool
(tool catalog checked). No native TodoWrite update is claimed. The dispatched
file contains no PLAN step 0; the explicit user request supplies step 0 above.

### Inputs and reproducible checks

- Reviewed by the assigned Codex REVIEW session, independently of the F21 memo author; no new worker was dispatched. Session/model backend identifiers beyond the supplied Codex identity are unavailable and are not invented.
- GitHub authenticated identity: `mcpdev80` (`gh api user --jq .login`).
- Live GitHub branch endpoints, checked twice: Core develop `1861ed07888a1ec22c218ca491d23ee54f48e261`; Planning main `c41cd2cf5d9d2ed7ba18bc346c0cd1ee4f2148e8`.
- Planning task at remote main remains `review`, owner `Codex (Maintainer complete v0.4.26 defect audit)`, blocker empty. Local `plan.py show` agrees. Its old OWNER STOP handoff is historical data, not authorization to resume implementation.
- SSH fetch failed before updating refs because `/etc/ssh/ssh_config.d/20-systemd-ssh-proxy.conf` has invalid owner/permissions. Live GitHub API branch/task/PR reads supplied the remote verification instead; no SSH settings changed. Local origin/develop and Planning HEAD already equal the live tips. Product working HEAD differs, so source inspection used immutable `git show 1861ed07:<path>` throughout.
- For each sampled full merge below: `gh pr view <PR> -R mcpdev80/baseharbor --json state,mergeCommit,mergedAt`; `git -C baseharbor merge-base --is-ancestor <merge> 1861ed07888a1ec22c218ca491d23ee54f48e261`; `git show <merge>` and immutable target source inspection. All three ancestry commands returned **0**.

### Three-finding sample

| Memo finding | Actual merge SHA and live state | Source evidence at C2-final | Result |
| --- | --- | --- | --- |
| 1 — CLI/MCP registration | [#1026](https://github.com/mcpdev80/baseharbor/pull/1026), MERGED 2026-10-21T15:04:51Z; `fe2d0c4a8c1df2d15058e338abb78adeb4cc9b83` | `cmd/baha/commands.go:241–242` registers Edit/Footprint; `mcp_tools.go:139,469` registers Footprint/Edit. `app_registration_921.go:95–102,121–124` retains snake-case transport and typed wrong-Target conflict; `registration_921_test.go:24,55,84,157,228` covers real command/discovery/schema/propose/negative paths. | CONFIRMED, IN-C2 |
| 27 — single-No terminal input loss | [#1035](https://github.com/mcpdev80/baseharbor/pull/1035), MERGED 2026-10-22T17:53:59Z; `078fdd0a57092f2649c92c93df901a70b3d2b11f` | `application_provider_preferences.go:270–278` preserves the cancellable terminal interface including `Name()`. `app_new_wizard_test.go:707` is the real PTY regression `TestAppNewWizardActualTerminalNoPreservesChecklistAndCancels`: one No, retained SQL/Cache selections, returned checklist and Esc cancellation. | CONFIRMED, IN-C2 |
| 28 — truthful pre-execution Edit result | [#1036](https://github.com/mcpdev80/baseharbor/pull/1036), MERGED 2026-10-22T17:56:42Z; `2adae47afddc299d06dbc34eb16a8e011c14b6dc` | `app_edit_apply.go:256–267` returns the original admission failure before presenting any execution receipt. `app_edit_apply_test.go:186,192,238` supplies Guided coverage for consent/authorization/read refusals without writes or claimed changes, plus actual partial execution. | CONFIRMED, IN-C2 |

Full parent-to-merge diff SHA-256 (`git diff <merge>^ <merge> --` raw bytes):

- #1026: `81e47891c79ba574a1cd359730366b32f85fbca67cbef4b31732a0e79bbc6c38`
- #1035: `00e911ba23c048fc745025ec5a03020a2863e1cff8763106aa10bddd8d190eeb`
- #1036: `be4d4e76cb956267649d28f4dc0e03c51275c4a4f8583d8e090cc1932cf8ef00`

Existing CI readback: #1035 run [39826312781](https://github.com/mcpdev80/baseharbor/actions/runs/39826312781) has passing wizard-app-new-tests, cli-guided-tests and PR Selected Checks; #1036 run [39826736084](https://github.com/mcpdev80/baseharbor/actions/runs/39826736084) has passing cli-guided-tests and PR Selected Checks. Runtime jobs are skipped. `gh pr checks 1026` reports no checks on its branch today, so this review claims no fresh #1026 CI/native verification. No tests or CI runs were started; this bounded source-reference sample reuses existing evidence.

### Verdict and coordinator handoff

**ACCEPT — the requested three-finding merge-reference spot check is confirmed.**
For this dispatched audit handoff, the memo is ready for the Coordinator's card-closure decision. This session does not close or accept the Planning card, merge anything, or assert complete release qualification. A three-item sample does not re-review the other 29 dispositions or recover retired native receipts. The original task's published acceptance criteria remain binding for actual closure.

Actionable recipe: no product correction is required for the sampled findings. Coordinator should attach this AUDIT to its closure evidence and preserve the outstanding final-source native/consumer/binary/gate qualification under core-923. Before actual closure, verify the original card's criterion map, required independent reviews/checks and published Planning render/check evidence. Stop closure if any mandatory original-card evidence is missing or if a changed target invalidates the references. Final qualification is not inferred from ancestry.

Non-blocking memo precision: row 10 names the #1030 planner activation, while bootstrap source integration is correctly identified separately in row 26 as #1033 `3be56628`; retain that distinction when publishing. The memo's old `blocked` state is superseded by the live `review` card, not silently rewritten in its historical section.

### repo-rule compliance and operational flags

ROLLE: REVIEW throughout. Only this existing ecosystem evidence file was edited; product files, Planning task ownership/status, branches and foreign dirty work were preserved. No implementation, spawn, merge, release, deployment or messages to other sessions. Root policy retains the Maintainer pause; conflicting historical resumption prose was not used as authority.

Turn-start basics flags were triaged within this bounded REVIEW scope: low /tmp space means no spawn; pool/idle/busy/coordinator-plan flags concern the dedicated Coordinator and do not authorize this reviewer to re-task/remove sessions, clean foreign files or edit its plan. Reminder backlog was inspected as operational data and left unacknowledged rather than claiming resolution. Event queue had already been consumed concurrently; a fresh check returned zero unread, and `events-check.py --ack` acknowledged zero. Those operational items remain with the Coordinator. Native TodoWrite remains unavailable as recorded above.

END RW-AUDIT-DONE
