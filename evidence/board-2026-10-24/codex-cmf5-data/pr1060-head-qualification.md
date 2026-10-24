# Core-1060 CI binding qualification for #1060

Owner-authorized immutable source inventory for the exact #1060 candidate. This does not waive required checks or claim independent review. Only the authenticated `develop` copy is trusted. Any candidate edit invalidates this record.

```json
{
  "repository": "mcpdev80/baseharbor",
  "target_branch": "develop",
  "reviewed_head": "9ec2528253e1d91c48f81985f7c696a663a29fd6",
  "files": {
    "scripts/pr-check-runner.py": "modified",
    "scripts/pr-gate-plan.py": "modified",
    "scripts/test_pr_gate_plan.py": "modified"
  },
  "file_hashes": {
    "scripts/pr-check-runner.py": "5fe7617cd9111774b76423c4850aefbe3503059a2da072ebce77b4a0cc9f2a8a",
    "scripts/pr-gate-plan.py": "6d20a39e6893241ea7ec8453ad91414ca9035e6014abc21b1c6a99bbb3c768a3",
    "scripts/test_pr_gate_plan.py": "be6da46d18169cee23678579ed7f5a4e8ab3e108f7e20999a23d9372a7b2388e"
  },
  "required_checks": [
    "pr-planner-tests",
    "ci-policy",
    "ci-executor-verifier-tests"
  ]
}
```
