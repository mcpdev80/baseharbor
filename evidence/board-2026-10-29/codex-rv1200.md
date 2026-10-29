ROLLE: REVIEW
Session: Codex (/root); UTC: 2026-10-29T08:33:53Z; dedicated independent review of named objects only.

## PLAN
- [x] 0. Read binding policies and exact dispatch; establish review-only scope and record this plan before candidate inspection.
- [x] 1. Fetch product and Planning remotes; freeze exact candidate/base and read applicable rules/task.
- [x] 2. Inspect candidate diff, trusted controller pin ancestry/content, post-run pin divergence and shadow pin.
- [x] 3. Verify live PR checks and Planning evidence claims against exact objects.
- [x] 4. Record independent verdict, reproducible findings and executable worker fix recipe; verify candidate unchanged.

Native TodoWrite is not exposed in this session (tool discovery performed); this file is the persistent checklist. Dispatch contains no literal PLAN step 0, so step 0 above records its prerequisites without inventing product work.
Basics flags: shared events/reminders, under-target worker pool and shared open PLAN steps. This dedicated REVIEW lane does not own controller event acknowledgements, pool scheduling or other lanes' plans; preserve them and do not start a controller. Existing root index has conflicts and unrelated changes; preserve all.


VERDICT: REQUEST_CHANGES

## Exact objects reviewed

- Product PR #1200, OPEN, head `5aaa38ece21c40ad10d2805bf0bc41fac3d695b8`, base develop `7a4debccc4e2fa5e78287e9b9388b0cb521d66e2`.
- Approval/shadow pin: `b53632aaaee6067397c27000c229f34736ec2155`.
- Prior shadow: `f57b30ed48f19e62452ab793801a02ccd2658414`.
- Actual post-run pin: `e13f1570550f4b6b09f3727728293d91afbebf94`.
- Dispatch's comparison pin: `dd8fb61d614b59252c9d29121380b5893d006828`.
- Planning HEAD and fetched origin/main: `d9c740a8f59d7d460f9120e3bec5e610f198bd4c`. Local approval-pin.md equals fetched remote evidence. Task owner remains o201; state review.

Initial fetches succeeded. After daemon recovery, SSH ls-remote failed on `/etc/ssh/ssh_config.d/20-systemd-ssh-proxy.conf` permissions; authenticated `gh api .../git/ref/heads/...` independently reconfirmed all three remote branch SHAs unchanged. No SSH/config repair attempted.

## F1 — blocking: configured pin cannot accept this approval workflow

Location: product `.github/workflows/receipt-approval.yml:88` (also misleading explanation at :30–36); enforced by `scripts/receipt_verifier.py:338–350`.

The pin contains the old approval workflow blob `af512987239ac36e1db823f473d91e900d6d1097`, while PR #1200 changes it to `2d00cb41` (prefix). `pinned_source()` compares the complete workflow bytes at TRUSTED_SOURCE with those at authenticated GITHUB_SHA. Consequently this candidate deterministically raises `running approval workflow differs from reviewed pinned definition`, before approving anything. Unchanged verifier bytes are insufficient. Changing comments or the shadow uses line also changes the verified workflow bytes. This is still fail-closed, but fails the task's positive acceptance criterion; setting a nonempty full SHA did not repair it.

Reproduction, from ecosystem root:

```bash
python3 evidence/board-2026-10-29/scripts/rv1200-pin-repro.py
```

Executed result:

```text
b53632aaaee6067397c27000c229f34736ec2155 PASS
5aaa38ece21c40ad10d2805bf0bc41fac3d695b8 BLOCKED: running approval workflow differs from reviewed pinned definition
```

The script reads the exact candidate verifier bytes using git show, loads an unchanged temporary copy, and calls the real pinned_source against the original product Git object database. This is a local model of the authenticated run SHA, not a claim of hosted identity or an attempted approval. No Planning writes, credentials or GitHub mutation occur. The old-pin positive control proves the failure is the changed workflow. No focused tests were added by this PR. Existing TrustedSourceTest fixtures at `scripts/test_receipt_verifier.py:704–738` test divergence rejection but do not establish that the committed workflow's configured pin succeeds.

## F2 — evidence correction required: uniqueness and successful wiring claims

Location: Planning `evidence/r27-ci-replacement-2026-10-29/approval-pin.md:28–34`, :65; PR body repeats the repair claim.

b53632aa really exists and is an ancestor of develop (`git cat-file -p <full pin>` inspected; `git merge-base --is-ancestor b53632aa origin/develop` returns 0). It carries the named verifier, writer and tasklink. Its verifier and shadow blobs equal develop's. However “exactly one reviewed develop head carries” these bytes is false as a content/ancestry argument: develop itself and later develop descendants carry the same blobs. `git log b53632aa..origin/develop -- scripts/receipt_verifier.py .github/workflows/receipt-approval.yml .github/workflows/receipt-shadow.yml` is empty; the unchanged object IDs persist throughout that interval. Presence or first introduction of bytes does not establish unique independent source qualification. No prior review ruling was reused to assert that qualification here. F1 directly contradicts the claimed operational pin repair. Replace these claims with the actual qualification provenance and successful exact-wiring test after correction.

Reproduce with `git rev-parse b53632aa:<path> origin/develop:<path>` for each of the three paths above, plus the F1 script. Evidence's test-pass history is reported history; this review does not independently attest that those local executions occurred. Its “CI in progress at time of writing” at :71 is explicitly historical, not a false green assertion. The task's evidence URL still contains literal `{sha}` (task note :60); replace it with the actual immutable Planning commit URL during the owner's next handoff.

## Controller pins and shadow assessment

The dispatch's assertion that receipt-post-run.yml pins dd8fb61d is stale for these exact objects. Candidate `.github/workflows/receipt-post-run.yml:86` and :219 both pin e13f157. dd8fb61d exists, but is not an ancestor of develop; neither is e13f157. All three comparison commits carry verifier blob `9cb8cb93ddfe62f5220f67b20764eaf9328302d3` and tasklink blob `52f11644dc72d3d38a71973fa0a73f7091f63c5c`.

Writer differs: b53632aa/e13f157/develop carry `9a0bcf8a146b5726bb026507f0f56a0d2536e7a3`; dd8fb61d carries `abf4d075c54371fde66dcb4468f55aa5dd512281`. `git diff b53632aa dd8fb61d -- scripts/receipt_verifier.py scripts/receipt_writer.py scripts/receipt_tasklink.py` shows only writer changes: dd8fb61d adds --product and forwards product through write_and_publish/main. Therefore two distinct SHAs are not intrinsically a defect: actual approval and post-run pins have identical controller module bytes. They serve different roles and need separate qualification. The alternate dd8fb61d contains a real CLI fix and cannot be described as byte-equivalent or silently substituted; it is outside this workflow-only diff. This review does not certify the complete hosted post-run chain. The demonstrated defect in PR #1200 is workflow self-binding (F1), not merely differing commit labels.

The old f57b30ed is a two-parent worker merge, not an ancestor of develop (ancestry returns 1). Its shadow has inputs receipt/plan/candidate only at :29–41 and a missing-input skip guard at :56; approval passes publication as well. b53632aa's shadow declares required publication at :46–49 and explicit input validation. PR's shadow repin at `.github/workflows/receipt-approval.yml:141` correctly fixes that interface mismatch. It pins the reusable workflow definition; shadow's verifier checkout still uses authenticated github.sha (:76). Pinning the reusable definition does not alone pin every executing script to b53632aa.

## Actual CI and acceptance mapping

`gh pr checks 1200 --repo mcpdev80/baseharbor` showed run `40839765984`, attempt 1: Trusted Plan, Focused Python, ci-actionlint, ci-policy, ci-executor-verifier-tests, pr-planner-tests, receipt-execution-tests, Workflow policy and PR Selected Checks all pass. Runtime/consumer/exact-candidate/release jobs shown as skipping remain skips, not asserted coverage. Run API binds success to exact head `5aaa38ece21c40ad10d2805bf0bc41fac3d695b8`. Check-runs API verifies PR Selected Checks success, app ID **15368**, job `123333872994`.

- Intended trusted source loads successfully: FAIL, F1.
- Wrong/absent pins rejected: existing checks remain fail-closed; no weakening observed.
- Shadow executes reviewed source: immutable reusable definition with corrected publication interface present; actual hosted execution not demonstrated here.
- Focused checks/workflow validation: live selected checks PASS, but no real configured-pin success regression exists.
- Hosted end-to-end: explicitly separate task proof; not claimed by this verdict.

## Executable worker fix recipe (not implemented by REVIEW)

1. Keep o201 ownership; reproduce F1 with the command above before editing. Do not replace the literal pin with another arbitrary historical SHA: any literal self-pin update changes the workflow it is expected to equal. A commit cannot ordinarily contain its own full Git commit SHA.
2. Coordinate a narrowly expanded write set for a separate committed pin configuration file (for example `.github/receipt-approval-source`) plus the workflow and focused tests. One concrete approach preserves the existing verifier: change receipt-approval.yml to load TRUSTED_SOURCE from that file after the authenticated checkout; validate exactly one lowercase 40-hex SHA and export it only for the verifier process. Retain all Owner/run/ref checks, credential controls, source equality checks and shadow full-SHA pin. Create reviewed commit A containing final workflow/helper bytes, then commit B changes only the configuration file to A's full SHA. A and B must have identical approval workflow and verifier bytes. Both commits are review candidates; do not activate or publish a workflow merely to test this.
3. If that extra path is not reserved/authorized, stop and ask the integration owner to reserve it or agree an equivalent independently reviewed non-self-referential source binding. Shared verifier changes require its existing owner's coordination; never remove the workflow equality guard just to make the test pass.
4. Add an exact committed-configuration positive regression in scripts/test_receipt_verifier.py: resolve the configured pin from final candidate, load candidate verifier, assert pinned_source succeeds against final candidate's workflow. Exercise missing, malformed, unavailable pins and changed verifier/workflow as negatives. For the two-commit method, create real temporary Git commits A/B in the test; do not stub source_blob or compare fixtures unrelated to the configured workflow. Test shadow publication interface as well.
5. In the assigned isolated worker worktree, run `cd scripts && python3 -m unittest test_receipt_verifier test_receipt_chain`; run `actionlint -shellcheck=false .github/workflows/receipt-approval.yml` locally if shellcheck is unavailable and rely on mandatory hosted lint for the full lint gate. Include the new configuration in the trusted CI impact selection as needed. Verify the actual configured-pin success and all negatives before a single coherent push. Obtain exact-new-head mandatory checks and independent review; old green CI does not qualify changed inputs.
6. Correct approval-pin.md F2 claims, record new SHA/test results and immutable handoff URL; run Planning render/check when publishing that handoff. Do not claim hosted-chain qualification from these unit tests.

Stop criterion: REQUEST_CHANGES persists until final committed wiring passes its positive source check, malformed/wrong source and changed bytes still fail closed, source qualification is inspectable, shadow input compatibility is proven, and final exact-head required checks pass. No merge/approve/product push performed. Product candidate and remote base rechecked unchanged through authenticated API at review close. Prior rulings were not used.

END RV1200
