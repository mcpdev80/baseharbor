# F33-PINSDEVELOP: docs/releases/v0.4.26.consumer-pins.json onto develop (only content, PR only)

Lane: f33 [wip:f33]
Prompt: evidence/board-2026-10-22/prompts/f33.txt
Owner-FREIGABE given in prompt: single develop content for v0.4.26 = the consumer-pins JSON. PR against develop, NO merge.

## PLAN

- [x] 0. Turn-start basics-check + events processed/acked (AGENTS.md Pflicht)
- [x] 1. Write this evidence file + native todowrite (first write action)
- [x] 2. pwd-check before EVERY git command (git FORBIDDEN in repo root); worktree baseharbor/worktrees/f33-pins on branch work/v0.4.26-consumerpins-develop (recovered existing q87 branch; see Findings)
- [x] 3. Schema model researched: `git show origin/develop:docs/releases/v0.4.25.consumer-pins.json` FAILS - that file exists on no ref; authoritative 26er schema = `scripts/ecosystem_release_pins.py` resolve_pins(release=0.4.26) output (23er envelope + ecosystem DAG)
- [x] 4. develop-tip SHA = 1861ed07888a1ec22c218ca491d23ee54f48e261 (origin/develop, base of the committing branch) -> core
- [x] 5. Created docs/releases/v0.4.26.consumer-pins.json in worktree via the script itself (core/demo/console/connector exactly as FREIGABE)
- [x] 6. Consistency: script run resolved console 69d1d140... from connector integration-candidate.json and proved demo baseharbor-core.ref == core 1861ed07...; validate_ecosystem_graph passed; all 3 pins gates are members of release-requirements.json v0.4.26-required integration gates; pins match q86-pinsfix.md consumer values
- [x] 7. Commit e551bc8b, pushed normal (fast-forward on q87 head), PR #1050 against develop auto-updated (KEIN Merge)
- [x] 8. END F33-PINSDEVELOP-DONE with pr + head + datei-praewiew

## Findings

- Prompt premise error (documented, not silently fixed): docs/releases/v0.4.25.consumer-pins.json exists on NO branch (develop, main, or any ref). Only v0.4.23.consumer-pins.json is committed. Schema therefore taken from the generator `scripts/ecosystem_release_pins.py` (branch-based), which for release 0.4.26 emits: `baseharbor.consumer-evidence-pins/v1` envelope + `gates` (docker/podman/live-console, live-console entries carry `origin`) + `ecosystem` four-node acyclic DAG carrying the core/demo values. Without the ecosystem section the prompt's core/demo values would appear nowhere.
- q87-pinsdevelop lane (stalled, no evidence file, session gone) had already created branch work/v0.4.26-consumerpins-develop, pushed f2434c82 and opened PR #1050 with a GATES-ONLY file: missing `ecosystem` section, missing `origin` fields, thus core and demo values absent and not the script-canonical form.
- Recovery per AGENTS.md ("Recovery bevorzugt ursprünglichen Owner/Branch"): continued on the existing branch (no force-push, no duplicate owner). Main baseharbor checkout was dirty-stranded on the work branch with clean tree and pushed head -> detached it to origin/develop (recoverable, no data loss) so the worktree add could check out the branch. This session 58915389 (f33-pinsdevelop) is the only busy agent; q87 session no longer exists.
- The f33 canonical file is generated deterministically by `python3 scripts/ecosystem_release_pins.py --tag v0.4.26 --core 1861ed07... --connector df1f437... --demo 25c3196... --output docs/releases/v0.4.26.consumer-pins.json` (console 69d1d14... resolved transitively, matching FREIGABE value).
- No commit exists "on top of which" the file could take a newer core: origin/develop tip 1861ed07 == the base used, so core stays 1861ed07. If develop moves before merge, the merged-tip core is re-qualified by pre-release CI (out of scope: prompt says PR only, KEIN Merge).

## CI status PR #1050 @ e551bc8b

- Run 40028406804: PR · Focused Python pass, PR · Validation pass, PR · Trusted Plan pass, Workflow policy pass;
  heavy product gates skipped (docs-only, per CI-efficiency rules). "PR Selected Checks" FAIL (exit 1):
  `BLOCKED: coverage-unresolved: unknown impact for docs/releases/v0.4.26.consumer-pins.json` +
  `no required gates or validations`. Identical failure on prior head f2434c82 (verified via check-runs API)
  -> pre-existing `scripts/pr-gate-plan.py` coverage gap for docs/releases/*.json, not introduced by this lane.
  Deliberately not patched here: owner-FREIGABE fixes the pins JSON as EINZIGER develop content; planner
  qualification belongs to the #1039/#1042 lineage (f19 precedent: "No qualification-map expansion").
  Documented on the PR as handoff comment 6375363149.

## repo-rule compliance

- pwd checked before EVERY git command; all git executed with `git -C .../baseharbor` or inside the worktree; no git in ecosystem root.
- Product work isolated in baseharbor/worktrees/f33-pins (branch work/v0.4.26-consumerpins-develop); main checkout
  only detached from stranded f2434c82 to origin/develop (clean tree, head pushed - no data loss).
- Normal push, no force, no merge, no tag/release/deploy. Claims protocol: coordinator-dispatched lane via prompt
  file with explicit Owner-FREIGABE; no separate plan.py claim published (board-2026-10-22 sibling-lane precedent).
- Local focused verification: `python3 scripts/ecosystem_release_pins.py ...` (generation = verification, graph
  validated) + `PYTHONPATH=scripts python3 -m unittest scripts/test_ecosystem_release_pins.py` -> 8/8 OK.
- File contains exactly the four FREIGABE values; console/connector/demo SHAs identical to q86-pinsfix.md;
  gates subset of release-requirements.json v0.4.26 required integration gates.

## Datei-Präwiew (docs/releases/v0.4.26.consumer-pins.json @ e551bc8b)

Schema `baseharbor.consumer-evidence-pins/v1` + `ecosystem` (0.4.26-only DAG):
gates: integration/docker/remote-target + integration/podman/remote-target -> connector df1f4378...;
integration/static/live-console -> connector df1f4378... + console 69d1d140... (each with docker-gate `origin`).
ecosystem: core 1861ed07... (consumes {}), demo 25c31969... (consumes core),
console 69d1d140... (consumes core+demo), connector df1f4378... (consumes core+demo+console).
File sha256: ba77fc3642913a68483485dca8067fed1fc3811c094d223ab4c6b26810788452.

## END F33-PINSDEVELOP-DONE

- pr: https://github.com/mcpdev80/baseharbor/pull/1050 (OPEN, base develop, MERGEABLE, KEIN Merge ausgefuehrt)
- head: e551bc8bf2be88d2720deaca1317160256d49760 (normal fast-forward onto q87 remnant f2434c82)
- datei-praewiew: as above; single file changed vs origin/develop (docs/releases/v0.4.26.consumer-pins.json)
- offener Befund fuer Coordinator: PR Selected Checks fail-closed wegen fehlender docs/releases-Abdeckung im
  Planner (pre-existing, recipe: planner lane, not this PR).

## Evidence log

1. basics-check FLAGS processed; events acked; reminders triaged (foreign lanes, diagnostics only).
2. sha256 of committed file @ e551bc8b: ba77fc3642913a68483485dca8067fed1fc3811c094d223ab4c6b26810788452
   docs/releases/v0.4.26.consumer-pins.json; origin branch head confirmed == e551bc8b; PR #1050 headRefOid == e551bc8b.
