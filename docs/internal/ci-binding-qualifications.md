# Core-921 CI binding qualification (#1056)

Owner-authorized immutable source inventory for the exact #1056 candidate. This does not waive required checks or claim independent review. Only the authenticated `develop` copy is trusted. Any candidate edit invalidates this record.

```json
{
  "repository": "mcpdev80/baseharbor",
  "target_branch": "develop",
  "reviewed_head": "9d1624e2b6641404d09cfd77cd92e250ec5e6dbf",
  "files": {
    "scripts/pr-check-runner.py": "modified",
    "scripts/pr-gate-plan.py": "modified",
    "scripts/test_pr_gate_plan.py": "modified"
  },
  "file_hashes": {
    "scripts/pr-check-runner.py": "5f255bc2ebd20c2d96112104788cbefbd2633001556ab9509e405196b12b26fe",
    "scripts/pr-gate-plan.py": "0218dbad0dc324795ef420e08705889be79700c05c5211b83e67c0e0f8616db2",
    "scripts/test_pr_gate_plan.py": "2ee96d581717b831da30f05099cd7efe707027f68bedd35168d4028ef88e39bc"
  },
  "required_checks": [
    "pr-planner-tests",
    "ci-policy",
    "ci-executor-verifier-tests"
  ]
}
```
