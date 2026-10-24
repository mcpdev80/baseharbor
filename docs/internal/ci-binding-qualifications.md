# Core-1060 CI binding qualification for #1060

Owner-authorized immutable source inventory for the exact #1060 candidate. This does not waive required checks or claim independent review. Only the authenticated `develop` copy is trusted. Any candidate edit invalidates this record.

```json
{
  "repository": "mcpdev80/baseharbor",
  "target_branch": "develop",
  "reviewed_head": "1c2f1fc476794b6da7c315082cf155b2d6e680bb",
  "files": {
    "scripts/pr-check-runner.py": "modified",
    "scripts/pr-gate-plan.py": "modified",
    "scripts/test_pr_gate_plan.py": "modified"
  },
  "file_hashes": {
    "scripts/pr-check-runner.py": "f71bf2cf1dac197ff0b998630c165c70f1c83a742e38245bfbc256f8c630c36d",
    "scripts/pr-gate-plan.py": "38967c37f3d11c5e5c392cc6c39e7ba5711be0f3c2bacad861b4b7b02ec8f673",
    "scripts/test_pr_gate_plan.py": "c291efdd8146a8d801ad747a59e889d5d6ecf91285829a7476c4788a37e285d7"
  },
  "required_checks": [
    "pr-planner-tests",
    "ci-policy",
    "ci-executor-verifier-tests"
  ]
}
```
