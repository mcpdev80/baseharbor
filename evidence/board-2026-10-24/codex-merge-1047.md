ROLLE: MERGE

## PLAN

- [x] 0. Read cmx1047.txt with cat, binding bootstrap policies and repository rules; establish bounded MERGE scope and record this plan before live candidate inspection.
- [ ] 1. Verify live account, exact PR head, checks, each review thread and merge state; stop on changed head or red/pending gate.
- [ ] 2. Verify live rulesets and exact-head independent ACCEPT; require mission's approval condition without self-approval or bypass.
- [ ] 3. Only if all gates pass, perform expected-head squash merge and verify remote develop and its checks; otherwise record BLOCKED.
- [ ] 4. Record read-only #1054/#1056 observation, UTC times, SHA/run receipts, compliance and final marker.

Native TodoWrite is absent from this session's tool catalog. No native TodoWrite execution or GUI completion is claimed. The mission contains no numbered PLAN step 0; the setup step above implements the user's additional request.

## Scope and constraints

Expected PR #1047 head: 79e0aa2b03f66e1a056976d018f4f97949e51417. Sole account mcpdev80. No product implementation, release, tag, deployment, protection change, administrative override, second account or unrelated lane management. The user explicitly requests this bounded mission; no general controller activation or lifting of the regular Maintainer pause is inferred.

The bootstrap retains stricter independent-review gates than historical repository delegation prose. Mission step 3 additionally requires Approved and forbids protection bypass. A local ACCEPT must not be presented as a formal GitHub approval. Prior codex-verdict-1047.md records GitHub rejecting self-approval; live state must be checked rather than repeating that mutation without new grounds.
