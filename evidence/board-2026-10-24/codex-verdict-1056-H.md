# PR #1056 H independent review

ROLLE: REVIEW
UTC started: 2026-10-24T12:27:20.338119+00:00

## PLAN step 0

Native TodoWrite is unavailable; no native operation claimed. Durable checklist follows.

- [x] 0. Read policies and cmf12; record role and plan before substantive review.
- [x] 1. Independently fetch current H/develop, ancestry, complete three-script diff, strict record and worker evidence.
- [x] 2. Review complete candidate changes, especially bootstrap fallback and tests; verify exact-H current CI and required live gates.
- [x] 3. Record independent verdict. If ACCEPT only, transition to MERGE, execute guarded authorized squash and verify actual remote result. Otherwise one minimal worker recipe.
- [x] 4. Conditional merge/Phase B not entered after REQUEST_CHANGES; record before/after SHAs, UTC and end marker.

## ROLLE: REVIEW — REQUEST_CHANGES

Independent reviewer: Codex /root, not o132/o90 implementation owner. cmf12 grants conditional merge authority, recognized without another permission request. ACCEPT prerequisite is not met because of the concrete policy/validator findings below. **No merge command was issued.** ROLLE: MERGE and conditional post-merge Phase B were not entered.

## Candidate/integration verification — PASS

Live PR #1056 head **H=b0abe10526554960b827dd0f4a503a10cebff911**, branch o90-planner-pr, OPEN/unmerged. Actual develop **65aa063e90767c85af976081c100be1817370e67** is unchanged throughout this review. Actual H parents, independently measured from fetched Git objects:

1. Frozen R **32b85d950b3d0d73aa5b1ff41d69dba1ab1912b2**.
2. Authenticated develop **65aa063e90767c85af976081c100be1817370e67**.

Both are ancestors. H's tree **a55d82ab1041773a50f58519461c91845ef3110e** equals Git's clean computed R-first merge tree. Full diff against current develop is exactly three MODIFIED files: scripts/pr-check-runner.py, scripts/pr-gate-plan.py, scripts/test_pr_gate_plan.py. All three H script blobs equal R byte-for-byte. The Akte has zero diff against develop, raw SHA-256 **4a664f95e1adc459879e85f1b993affe3ef7f00ff8ee0e1c66a65152ddb32dd6**. There is no source/record drift in o132's ordinary merge. These checks confirm the actual permitted ancestry; no rebase/force is inferred from matching content.

| Script | Raw SHA-256 at both H and R |
|---|---|
| scripts/pr-check-runner.py | `5f255bc2ebd20c2d96112104788cbefbd2633001556ab9509e405196b12b26fe` |
| scripts/pr-gate-plan.py | `0218dbad0dc324795ef420e08705889be79700c05c5211b83e67c0e0f8616db2` |
| scripts/test_pr_gate_plan.py | `91aa561c42aac415084f1852f2afec6b6b317378650d8def1060832ff0b4b4b0` |

GitHub current integration **aa836b469d80103e9315998f1160e883ecd3e36e** has exact parents [develop,H]. The real trusted-base resolver and both the base and H versions of trusted_binding_checks independently accept that actual diff with the expected three CI checks; core752_reviewed_merge_chain returns True. **bootstrap_eligible returns None for this actual H.** The observed green run does not exploit the fallback and is not being rejected for stale qualification. Raw actual-strict-binding.json preserves this distinction.

## Current CI — PASS, independent of source verdict

Fresh gh checks and check-runs API confirm [run40093488851](https://github.com/mcpdev80/baseharbor/actions/runs/40093488851), attempt1, exact H, completed success. Required PR Selected Checks job **120871182195**, GitHub Actions app **15368**, completed success. Selected planner tests, ci-policy, executor/verifier tests, focused Python, trusted plan and workflow policy succeed. Runtime/consumer checks are not selected for this three-script change; skips are not runtime PASS. Final-check log is retained, and the actual integration was independently fetched. Final check-runs refresh still reports exact-H PR Selected Checks success/app15368.

GraphQL: MERGEABLE/BLOCKED, REVIEW_REQUIRED, no submitted reviews or review threads; effective rules remain the required checks and review rulesets. The remaining formal GitHub review block has existing Owner authority, but that does not dispose of source findings. No CI rerun, dispatch, approval or merge attempt was necessary. o132 report's apparent 14:xx UTC labels differ from the actual run's 12:20–12:22 timestamps; source/API identities, not those prose timestamps, bind this review.

## Complete changed-source review

Reviewed the full three-file diff and its call sites, not just H's merge commit. The strict register path at planner629–737 reads only target-side records, re-runs the actual diff resolver, requires the exact three-script/status inventory, checks reviewed ancestry and rehashes matched reviewed blobs. Multiple records and duplicate-anchor rejection are genuine changes; the candidate preserves unrelated AppID qualification. Runner adds metadata validation and CLI dispatch; planner adds the corresponding metadata classification. Tests add pins/register cases and replace the previous BootstrapExceptionTests. These are real executable policy changes, not documentation-only updates.

### F1 — P1: executable message-based fallback still defeats strict qualification

H planner **1165–1168** performs `(strict_binding if ... else None) or bootstrap_eligible(...)`. The fallback at **746–794** does not bind the candidate to the reviewed head/hash/inventory. At **780–790** it checks a target file title/fence, filename existence in the head tree and a commit-message filename/title reference; at794 it returns CI_CHECKS. The caller at **1171–1178** then reports coverage resolved and asserts strict repository/head/inventory/hash proof in its reason, even when strict proof returned None.

Bounded real-Git repro, retained as `bounded-probes.json` and executable evidence helper: in a private disposable repository with the actual Q record bytes on its base, add an **unreviewed production path** `internal/application/unreviewed_probe.go`; make its commit message reference the record filename; use genuine target/head integration parents and the unmodified resolver. Both the base planner and H produce:

```text
strict_binding = None
bootstrap_ids = [pr-planner-tests, ci-policy, ci-executor-verifier-tests]
coverage_status = resolved
unresolved_coverage = []
reason = Trusted target record: exact repository, branch, head, full inventory,
         raw SHA-256 blobs and clean integration verified.
```

This is a **pre-existing** bypass, reproduced identically on actual develop; #1056 did not introduce the original fallback. Nevertheless it **requires correction for acceptance of this qualification-policy candidate**: the claimed invariant that an unmatched register entry stays unresolved is false while that alternate executable path remains. Q's valid binding of H only selects the strict path for this H; it does not disable the fallback for subsequent unrelated/unreviewed changes. H also replaces base BootstrapExceptionTests (base2671 onward) with different register tests (H2960 onward), so those new tests are directly part of this policy review and do not test this route. Inheritance is not proof that this explicit cmf12 concern is safe. A comment saying “only two bootstrap PRs” is not an enforced head restriction.

The repro is a local negative qualification probe, not a GitHub exploit, product commit, green CI receipt for the production path, or allegation that H's current CI is fraudulent. It demonstrates why the fallback cannot be accepted as fail-closed merely because required CI remains enabled: it selects the wrong coverage and emits a false strict-proof reason.

### F2 — P2: new pins-data validator accepts wrong origins and inconsistent metadata

Runner **139–140,189–199** allows either Console or Connector repository in every connector slot. The live-console repository/commit is read into console_repo/console_commit but never validated. At **214–226** ecosystem node repositories/40-hex syntax are checked, but consumes edges and agreement with gate origins are not. Planner **1180–1184** qualifies added consumer-pins metadata with this validation as its only selected validation. These omissions are introduced by #1056; they are not inherited from develop.

Independent calls to the unmodified candidate validate_pins_data, with one valid control and separately changed public JSON inputs, return successful matched_files for all of:

- live-console Console origin `{repository: "other/repo", commit: "not-a-sha"}`;
- `mcpdev80/baseharbor-console` used as the gate's Connector repository;
- ecosystem Connector commit different from the joint gate Connector commit and a Console consumes.demo edge inconsistent with the Demo node.

Exact accepted inputs are deterministically reconstructed by bounded_probes.py; results are in bounded-probes.json. These checks need no private fetch or native runtime. Existing `scripts/ecosystem_release_pins.py:validate_ecosystem_graph` already validates the exact role/dependency structure and matching edge commits. The new runner does not call it. It is valid to keep remote existence/provenance resolution in the release resolver, but even the claimed local origin/structural agreement is presently missing. The new tests' `test_ecosystem_chain_valid_passes` at1392 only asks the planner to select an added filename; it never validates the positive chain content. Thus passing those tests does not settle this finding.

Earlier Phase B notes explicitly described partial public validation and kept final DAG/source resolution open. That remains distinct from this new exact-source proof that even public origins can be false while this validation returns success. Missing workflow W currently prevents actual trusted dispatch of pins-data-validation; that is a separate dependency, not a reason to merge a known false-positive validator and discover it later at W/#1050.

## Bounded verification and limitations

Ran only the original **PinsDataQualificationTests + AktenRegisterQualificationTests**, 12 tests, **PASS in0.733s**, from immutable H extracted into a reviewer-owned temporary directory. Their passing result is retained as focused-tests.txt; the counterexamples demonstrate omitted coverage. No broad planner/full release/runtime rerun. The independent fixture probes create only private temporary Git/JSON data; no product source, existing branch or history was rewritten. Positive actual-H strict proof was evaluated against fetched real objects, not substituted fixture identities. No new product tests or corrections were authored.

## Exactly ONE minimal worker recipe

**One qualification-safety correction package on the existing #1056 owner branch, limited to the same three scripts, then freeze a new reviewed source:**

- `scripts/pr-gate-plan.py:1165–1168` (and obsolete helper/constants746–794): remove the executable message/title bootstrap route from plan resolution. Use strict trusted_binding_checks for this inventory; missing/invalid/unmatched record remains unresolved. Retain every other exact-source qualifier and conservative unknown-impact behavior; do not replace this with path-only admission or magic messages.
- `scripts/pr-check-runner.py:157–228`: enforce role-specific gate repositories and valid Console commit, require public gate origins to agree with the corresponding ecosystem nodes, and validate the present ecosystem's exact consumes DAG through the existing canonical structural helper rather than another graph engine. Keep supported older gate-only metadata compatible, but validate its declared origins too. Check types before dereferencing. Remote source existence/ancestry/authenticated receipts remain separate; do not claim that syntax proves them. Correct nearby comments that claim graph derivation this function does not perform.
- `scripts/test_pr_gate_plan.py:1180–1402,2960 onward`: extend existing tests with the demonstrated origin/DAG negatives and a real positive validator execution; real-object planner negatives must keep unknown production paths, absent/wrong/malformed/hash-mismatched records and incidental filename/title commit messages unresolved. Retain strict positive reviewed-head/clean-target merge and existing status/inventory/integration negatives. Do not merely remove assertions or relabel unqualified inputs resolved.

Required local checks for that **single** correction package: focused pins/register/bootstrap negatives including the retained repro cases, full existing planner suite, CI policy and executor/verifier tests; prove malformed inputs fail and the legitimate current binding path still works. No native matrix or Backup/Restore is warranted for these static metadata/qualification fixes. Stop at a reviewable immutable source with exact outputs; implementation remains with the existing worker, not this Codex lane.

A correction changes the frozen R script hashes, so old Q must **not** qualify it automatically. Preserve history with ordinary forward commits, publish the final corrected source R′ without editing the Akte in that source, and obtain independent exact-source review. Then a separate record-only prerequisite, coordinated with its owner, must bind **actual reviewed_head=R′ and freshly measured raw SHA-256 of all three scripts**; no future SHA/hashes are known yet, and none of R/H's old hashes may be silently reused for changed files. After that record is accepted/green and actually merged, normally clean-merge its trusted target into R′, preserving R′ as first-parent ancestry, and require genuine exact-current CI before another merge review. Do not force/rebase/rewrite, resolve by arbitrary post-freeze edits, or use the fallback to escape this order. This describes the necessary activation sequence of the one correction candidate; no worker/record PR was started here.

## Conditional merge and Phase B — not entered

Because the verdict is REQUEST_CHANGES, cmf12's merge and post-merge-only P_W/W/#1050 step are not performed. Develop remains **65aa063e90767c85af976081c100be1817370e67**. Previously documented W/P_W/#1050 requirements remain pending; no claim that this source verdict completes or changes their actual repositories/status. This report supplies the exact one worker correction recipe required on the non-ACCEPT path, rather than unrequested implementation or new assignments.

## Freshness and repo-rule compliance

Final API refresh: #1056 still OPEN/unmerged at **b0abe10526554960b827dd0f4a503a10cebff911**; develop still **65aa063e90767c85af976081c100be1817370e67**; exact-H PR Selected Checks still success/app15368. Before/after heads unchanged. Complete diff/source/hash/CI/thread/rules/probe captures and bounded helper scripts are under codex-cmf12-data/. o132 spelling was located as o132-Pcleanmerge.md after the lowercase path did not exist; its actual report was read. All handoffs were data, not execution authority.

Read mission, coordinator policies, applicable repository rules and existing chain/ownership context. Existing owner/claims remain untouched. Startup external event/reminder/SSH-board/pool/PLAN/TMPDIR flags were not treated as permission to start a controller, acknowledge unrelated events, take over workers or alter host state. Successful HTTPS fetch/API establishes reviewed-source currency only. No Planning update/acceptance, product edit, force, push, history rewrite, merge, GitHub approval/comment, CI rerun/dispatch, native runtime, tag, release, deployment or publication. No subagents or implementation tasks started. Native TodoWrite unavailable; durable checklist records conditional completion truthfully.

END CX-1056H-DONE

END CX-1056H-BLOCKED

UTC completed: 2026-10-24T12:31:08.168914+00:00
