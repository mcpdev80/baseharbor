W4A BOOT: worktree w4a created at /home/marcel/dev/baseharbor-ecosystem/worktrees/w4a, branch work/r27-fix-f1b-recovery, head f57b30ed48f19e62452ab793801a02ccd2658414 (verified = reviewed candidate head). LANE w4a CODEX-REVIEW worker, F1/F3/F5/D3 recipes.
## Turn checkpoint 2026-10-27 (w4a)
Pushed branch work/r27-fix-f1b-recovery @548db8b8 (verified origin).
- F1 writer: extract_facts wired into live() product check; --product arg.
- F1 publish(): _push_authed — in-process http.extraheader bearer only during push subprocess, token never persisted/logged, anonymous fetch read-back; _scrub_secret redacts bearer from stderr; _is_real_remote gates bare (tests) vs network (prod).
- D3: receipt_verifier.extract_facts(strict types, pos-int run_id/attempt/workflow_id, 40-hex head/base/integration, bounded artifact <=65536, duplicate-key via decode); 18 D3ExtractionNegativesTest pass.
- Policy: test_ci_workflow_policy_receipts job-level->if-guard; all 5 pass.
- OWNER findings (task card t-20261022-r27-evidence-publisher.md, Addendum push-credentials/job-hygiene): #1 push-auth in-process (DONE, +missing-token-fails-closed test), #2 base_ref indentation (in progress), #3 rename verify_producer->producer_context (in progress).
