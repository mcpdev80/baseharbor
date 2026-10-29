# Coordinator 2026-10-29 — v0.4.27 CI replacement, autonomous restart run

Lane: w2-baseharbor (2713f9f2, Build · Qwen Flash) — Coordinator-Session.
Order: baseharbor-plan/evidence/r27-ci-replacement-2026-10-26/CODEMAN-START.md (Owner restart 2026-10-29) + Owner directives this session (no timebox, no legacy fallback, ruleset only after proof).

## PLAN

- [x] Reality check: remote heads, PRs, runs, rulesets, claims, runners
- [x] Producer #1199 verified: merged 7a4debcc, run 40833893562 with 'PR · Public Contracts' success + trusted-pr-plan artifact (id 12880180956) + evidence artifact pre-release-integration-static-public-contracts-evidence-40833893562-1. Contracts file changed: contracts/contracts_test.go. #1188 is docs-only -> not a contracts producer; fails with integration_sha_mismatch (stale base after #1199 merge; per RUNBOOK §8 Frischkur, but out of producer scope).
- [x] Worktree registry 96 -> 83 (removed only clean+merged or clean+pushed-identical; 47 LOCALONLY worktrees preserved, RUNBOOK §13 hazard reduced, still >60 -> residual risk noted)
- [x] Watcher daemon re-bound to live coordinator session (old pid 1776122 -> new 1107475; dead 41e61806 entries removed)
- [x] Claims reassigned (planning commits: "reassign four r27 CI slice claims", "refresh generated views"): o201=approval-pin, o202=coverage-inventory, q201=cutover-design, q202=producer-diagnostics; push to plan main ok, check=0 errors
- [x] All four lanes spawned, model-verified at spawn, dispatched (pane+server busy proof), watchers registered
- [ ] Collect lane results per END events, disk-verify evidence before acceptance (qwen ghost rule)
- [ ] Shadow-run chain recipe execution (RUNBOOK §12) once pins + producer gaps are closed
- [ ] Cutover design review + ruleset patch ONLY after candidate-bound proof
- [ ] Ready-display cleanup for product cards (core-390/750/766/815) — no feature-ready before CI cutover
- [ ] Codex-REVIEW at exact SHAs, then Codex-MERGE per protocol

## Findings (Betrieb)

1. pool-admission.py LIMITS flash=1 counts the coordinator session, so a dedicated Flash-WORKER lane cannot be admitted while the coordinator is flash-built (f60 ADMISION-HOLD). Owner 2026-10-29 remapped hosts (ornith=H1, qwen=H2, flash=H3 for scaling); guard semantics need a coordinator-exclusion fix — documented, not silently worked around by config edits.
2. After the owner's remap, pane footers vs. lane names drifted: 1138dbc9 (o202) shows Qwen 3.6, 38460858 (q201) shows Qwen Flash, a71d5cd0 (q202) shows Ornith 1.5. Missions are model-independent; qwen-file-ghost rule applies by ACTUAL model, so all lane evidence gets mandatory disk checks here.
3. develop ruleset 'develop - required checks' id 25223521: context 'PR Selected Checks' (integration_id 15368), strict, enforcement=disabled. main ruleset active. Cutover = activate new check mapping after proof, per task contract.
4. PLANNING_RECEIPT_WRITE_TOKEN existence check delegated to o202 inventory (names only, no values).
5. Receipt chain state: only receipt 40428162731-1.json (2026-10-27) on plan main; run 40833893562 (merged producer #1199) has NO published receipt yet -> chain not armed (TRUSTED_SOURCE empty on develop; approval workflow dormant). q202 verifies this from the host.
6. #1168 (docs shadow-probe3) fails PR Selected Checks; docs-only branch, not part of producer path — keep open until inventory classifies it.

## Events log

- 07:5xZ session takeover, basics-check flags processed, events/reminders acked
- ~08:0xZ lanes o201/o202/q201/q202 spawned+verified; qwen spawn of q202 admitted despite q201 (guard counts 1/2 — reservation timing, benign)
- ~08:1xZ dispatch: 4x SENT-UNVERIFIED rc=6 then all four confirmed busy via agent list (delivery proven, no resend)

## Iteration 2 (10:0x-11:3xZ)

- #1201 MERGED (80d03122) | #1200 MERGED (a3750a8f, diff-guard 8d5bc939 beidseitig, rv1200fullB ACCEPT complete) | #1202 MERGED (f95b3446, rv1202b ACCEPT, merge1202 diff-guard 0b8e1067 PASS)
- #1203 (producer slice2) RV1203 REQUEST_CHANGES: go.mod/go.sum im Diff => coverage-unresolved + schwache Negative-Cases + Profil-vs-Extension Scope-Frage -> Fix-Runde o211
- #1204 (main-Registration, base=main, head 3fc45250) von q206 eroeffnet; rv1204 (Codex) + rv1204-worker (q206b) parallel
- Karten: approval-pin CLOSED (resolution accepted, 4 Evidence-Links); producer/postrun Karten synchronized; {sha}-Platzhalter entfernt (Owner-Review Punkt)
- RUNBOOK aus 8bec9ceb zurueckgerettet (237Z -> 251Z mit Rotation/Terra/TEMP-0/Preflight-Falle); Root-Repo laeuft weiter in Alt-Merge-Situation, keine Root-Commits
- coord-tick v2 live; Rotation-Guard in pool-admission.py (2 Layer-Checks, mit echten PIDs getestet); flash temporair 0
- Owner-Bewertung 2026-10-29 eingearbeitet: Preflight-Falle dokumentiert, Kartensync修复, Aufwand/Nutzen-Review nach erstem End-to-End-Lauf zugesagt
