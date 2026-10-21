# Core-921 CI binding qualification (#1007)

Owner-authorized immutable source inventory for the corrected #1007 candidate. This does not waive required checks or claim independent review. Only the authenticated `develop` copy is trusted. Any candidate edit invalidates this record.

```json
{
  "repository": "mcpdev80/baseharbor",
  "target_branch": "develop",
  "reviewed_head": "480060640e825c81b9e70e4a74df339cebcec36e",
  "files": {
    "scripts/pr-check-runner.py": "modified",
    "scripts/pr-gate-plan.py": "modified",
    "scripts/test_pr_gate_plan.py": "modified"
  },
  "file_hashes": {
    "scripts/pr-check-runner.py": "ec6b22d98eb9191d170af5149d489add136ec3d2d692d4e1008cb18f616b76ed",
    "scripts/pr-gate-plan.py": "946a8073b972af49853032c04ca6ebcac867f8b9832425cc34e7424a6d67d5e1",
    "scripts/test_pr_gate_plan.py": "ba9097c97e47d04a562050fd622fd00f8c8894a98e0287a048a9dbd44e51e4d2"
  },
  "required_checks": [
    "pr-planner-tests",
    "ci-policy",
    "ci-executor-verifier-tests"
  ]
}
```
