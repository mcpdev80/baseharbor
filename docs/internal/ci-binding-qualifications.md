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

## OpenBao retry CI source qualification (#1060)

Independent source review by /root/independent_review approves the exact integrated source below. The four-file tree preserves accepted policy #1064 and the corrected #1060 executor/workflow route. Review: full net source diff, preservation of both regression matrices, 15 focused integrity and execution tests passed. Coordinator complete planner suite: 121 tests passed. This target-side record authorizes only that immutable source and the listed checks; final mandatory CI and native evidence remain separate.

```json
{
  "repository": "mcpdev80/baseharbor",
  "target_branch": "develop",
  "reviewed_head": "7e08fe920d3f62219a3923669896c601eb43ae96",
  "files": {
    ".github/workflows/ci.yml": "modified",
    "scripts/pr-check-runner.py": "modified",
    "scripts/pr-gate-plan.py": "modified",
    "scripts/test_pr_gate_plan.py": "modified"
  },
  "file_hashes": {
    ".github/workflows/ci.yml": "3fdac135e92e99f636456e27e9e6c413dcfd1dbf3e631682a41485f96150dd26",
    "scripts/pr-check-runner.py": "f77fa0b195bdc3870279e0190c7c2f64d95d99928f134b53620e83a7d20731dd",
    "scripts/pr-gate-plan.py": "5f8eb479b4d4d580ccb6d3a08e4f37fc10d0ab49f5eece80153579a844e35263",
    "scripts/test_pr_gate_plan.py": "77e834474b37eb37505f32cd1f3878403a57c6628f5a51115e1e2038702302da"
  },
  "required_checks": [
    "pr-planner-tests",
    "ci-policy",
    "ci-executor-verifier-tests",
    "ci-actionlint"
  ]
}
```
