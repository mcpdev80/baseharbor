**CORRECT.** Moving the pins into a PR-controlled JSON file removes self-reference but does **not** establish trust. A follow-up commit can change the scripts and their JSON pins together while retaining `48def9e8` ancestry. That ancestry does not prove review.

My previous ruling required the qualification record to live **outside the candidate’s control**, in trusted base policy.

Use this concrete guard set:

1. Freeze the independently reviewed final candidate **H**.
2. Publish its qualification through a separate, independently reviewed prerequisite PR to trusted base policy. Record repository, target branch, H, complete path/status inventory, full file hashes and required checks.
3. Load that record exclusively from the authenticated trusted base revision. Candidate-provided JSON cannot supply authoritative pins.
4. Require `head_sha == H`. Alternatively, permit only the existing verified clean merges of trusted target commits onto H; reject linear follow-up edits.
5. Verify the complete actual diff inventory and reviewed content. For permitted target merges, prove every merge tree using the existing clean-merge verifier.
6. Preserve exact integration parents `[target_base, head]`, the expected clean integration tree and all mandatory executed checks. Missing or mismatched evidence fails closed.

**Bootstrap sequence:** freeze H → independently review and merge the trusted-policy prerequisite with green mandatory CI → evaluate frozen #1007 using that trusted policy. The prerequisite can contain H’s hashes because it is a different candidate; there is no self-reference.

Therefore:

- **Do not authorize pins from `scripts/pr-gate-plan-binding.json` at PR HEAD.**
- **Do not replace exact qualification with ancestry plus self-declared hashes.**
- The stale `48def9e8` equality must be replaced by the independently reviewed qualification for H, read from trusted policy.

This corrects the proposed design; it does not grant another implementation round beyond the existing correction budget.

RULING-3A-DONE
