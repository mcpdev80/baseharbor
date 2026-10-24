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
    "scripts/pr-check-runner.py": "34fe168fedbdcd503d0e026715372da466052853",
    "scripts/pr-gate-plan.py": "b0efbdc7ca354574316aa6b1d38517f9280ffdf6",
    "scripts/test_pr_gate_plan.py": "634b0ffc9311d14a17d698162b339a3e52922e43"
  },
  "required_checks": [
    "pr-planner-tests",
    "ci-policy",
    "ci-executor-verifier-tests"
  ]
}
```
