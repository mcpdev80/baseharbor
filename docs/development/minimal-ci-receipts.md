# Minimal CI receipts

Normal CI, Pre-Release and Release execute their required tests directly. Failing, cancelled or missing required jobs block the workflow through its normal dependencies and final result check. CI does not download gate manifests, reinterpret completed Actions runs or use an additional evidence or receipt approval chain.

## When checks run

Ordinary branch pushes do not start CI. Draft PRs run only workflow policy, source formatting and readability checks. Marking a PR ready runs the selected checks; later pushes to a ready PR rerun the affected checks. Bundle local commits before pushing. A merge into `develop` writes the receipt without a duplicate Source & Quality run. Explicit validation branches and manual validation remain available.

A valid changed path without a narrow mapping selects full Go unit tests, vet and build; changes in product/runtime areas additionally select Docker/Podman lifecycle checks. Invalid source identities, diffs, paths or catalog configuration still fail. The fallback never automatically starts the complete release matrix.

## Commit record

After an authorized merge into `develop`, `receipt-post-run.yml` writes one canonical JSON file to `baseharbor-plan`:

```json
{"commit":"<full merge commit SHA>","pull_request":123}
```

The path is `evidence/receipts/commits/<merge-commit-sha>.json`. The two fields are the complete record: no run IDs, attempts, job lists, PASS payload, manifest, approval or copied test output. The historical directory name does not introduce an evidence dependency. A receipt records the merge; it is not another check or release authorization.

## Publication

Only trusted default-branch code receives the Planning write credential. The writer authenticates the merged PR and its actual commit in `develop`, then publishes by a normal fast-forward push. It never queries CI runs, jobs or gate artifacts. Credentials must not appear in output.

Identical retries are harmless. A conflicting record or failed push fails publication; concurrent Planning changes are preserved. A missed merge event can be recovered using the existing workflow's `pr_number` dispatch input on `main`. Recovery uses the same writer and never rechecks CI.

## Test and release boundaries

Normal checks remain mandatory in the development process. The final CI result uses the current workflow's job results; it must reject failing or missing required work and must not turn a skipped required check into success. Source identity, immutable consumer pins, runtime isolation, real test execution and cleanup remain required.

Rulesets stay disabled while the CI correction is in progress. Their eventual activation is a separate operation. Pre-release execution, promotion, tagging and publication still need their authorized release scope; a commit record authorizes none of them.

Older pilot receipts remain historical data. The former live/approve/retained verifier, shadow consumer, automatic evidence links and gate-manifest reuse are retired.
