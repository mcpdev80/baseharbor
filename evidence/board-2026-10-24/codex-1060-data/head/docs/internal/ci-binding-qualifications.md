# Core-1060 CI binding qualification for #1060

Owner-authorized immutable source inventory for the exact #1060 candidate. This does not waive required checks or claim independent review. Only the authenticated `develop` copy is trusted. Any candidate edit invalidates this record.

```json
{
  "repository": "mcpdev80/baseharbor",
  "target_branch": "develop",
  "reviewed_head": "3fa4989e337c6c746acfb1be7ac3c0edad6bfdd8",
  "files": {
    "scripts/pr-check-runner.py": "modified",
    "scripts/pr-gate-plan.py": "modified",
    "scripts/test_pr_gate_plan.py": "modified"
  },
  "file_hashes": {
    "scripts/pr-check-runner.py": "5fe7617cd9111774b76423c4850aefbe3503059a2da072ebce77b4a0cc9f2a8a",
    "scripts/pr-gate-plan.py": "f60144461ddcb580868bb48f7a6c99c1b2990ac29cc7e471920aba21195d3fd2",
    "scripts/test_pr_gate_plan.py": "10713630a3066179366c109770177d864277aa9ab7cd6eafaa8ede414cf045fd"
  },
  "required_checks": [
    "pr-planner-tests",
    "ci-policy",
    "ci-executor-verifier-tests"
  ]
}
```
