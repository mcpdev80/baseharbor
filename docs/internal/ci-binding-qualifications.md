# Core-921 CI binding qualification (#1056)

Owner-authorized immutable source inventory for the exact #1056 candidate. This does not waive required checks or claim independent review. Only the authenticated `develop` copy is trusted. Any candidate edit invalidates this record.

```json
{
  "repository": "mcpdev80/baseharbor",
  "target_branch": "develop",
  "reviewed_head": "32b85d950b3d0d73aa5b1ff41d69dba1ab1912b2",
  "files": {
    "scripts/pr-check-runner.py": "modified",
    "scripts/pr-gate-plan.py": "modified",
    "scripts/test_pr_gate_plan.py": "modified"
  },
  "file_hashes": {
    "scripts/pr-check-runner.py": "5f255bc2ebd20c2d96112104788cbefbd2633001556ab9509e405196b12b26fe",
    "scripts/pr-gate-plan.py": "0218dbad0dc324795ef420e08705889be79700c05c5211b83e67c0e0f8616db2",
    "scripts/test_pr_gate_plan.py": "91aa561c42aac415084f1852f2afec6b6b317378650d8def1060832ff0b4b4b0"
  },
  "required_checks": [
    "pr-planner-tests",
    "ci-policy",
    "ci-executor-verifier-tests"
  ]
}
```
