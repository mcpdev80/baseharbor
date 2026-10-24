# Core-921 CI binding qualification (#1056)

Owner-authorized immutable source inventory for the exact #1056 candidate. This does not waive required checks or claim independent review. Only the authenticated `develop` copy is trusted. Any candidate edit invalidates this record.

```json
{
  "repository": "mcpdev80/baseharbor",
  "target_branch": "develop",
  "reviewed_head": "4e737e26af38e7f712f2de1f631633d8d59477b9",
  "files": {
    "scripts/pr-check-runner.py": "modified",
    "scripts/pr-gate-plan.py": "modified",
    "scripts/test_pr_gate_plan.py": "modified"
  },
  "file_hashes": {
    "scripts/pr-check-runner.py": "05f244d6e8de34a1f56a1123ce2a8ffb178943f064ef5cad1ed293f0a388517f",
    "scripts/pr-gate-plan.py": "7c285725156a0b08401dec6e18b11344795f5ed11c931230ef6bd2db0df769e8",
    "scripts/test_pr_gate_plan.py": "8564c6cb94b3fe035626f05b7b5d3c3cac584f354c43bcd37d80f86c9c86a010"
  },
  "required_checks": [
    "pr-planner-tests",
    "ci-policy",
    "ci-executor-verifier-tests"
  ]
}
```
