# Core-921 CI binding qualification (#1007)

Owner-authorized immutable source inventory for the exact #1007 candidate. This does not waive required checks or claim independent review. Only the authenticated `develop` copy is trusted. Any candidate edit invalidates this record.

```json
{
  "repository": "mcpdev80/baseharbor",
  "target_branch": "develop",
  "reviewed_head": "85e05c1c45f77da2d1d7227d298182bd867ceba9",
  "files": {
    "scripts/pr-check-runner.py": "modified",
    "scripts/pr-gate-plan.py": "modified",
    "scripts/test_pr_gate_plan.py": "modified"
  },
  "file_hashes": {
    "scripts/pr-check-runner.py": "ec6b22d98eb9191d170af5149d489add136ec3d2d692d4e1008cb18f616b76ed",
    "scripts/pr-gate-plan.py": "c297fe556690062684b6532c966657459e895f3816f02ca0de7c0209b9427cea",
    "scripts/test_pr_gate_plan.py": "1cdd0b2e65f5e2facb1ad51c4281c3dd03df8e4cc2f1a0f003f3d703fb2a24a6"
  },
  "required_checks": [
    "pr-planner-tests",
    "ci-policy",
    "ci-executor-verifier-tests"
  ]
}
```
