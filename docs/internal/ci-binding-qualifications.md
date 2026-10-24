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

## Canonical pins metadata qualification (#1056)

Independent source review by /root/independent_review approves the exact four-file source below. It removes the rejected bootstrap fallback and validates metadata through the canonical graph, pins resolver and private-origin owners extracted from the trusted base. The complete 150-test coordinator selection passed; the independent reviewer additionally passed eight targeted execution, source-binding and fail-closed tests. Final required CI remains mandatory after trusted-target integration.

```json
{
  "repository": "mcpdev80/baseharbor",
  "target_branch": "develop",
  "reviewed_head": "ae3f3ae20a56f406e9c461f405bb60d463658aa2",
  "files": {
    ".github/workflows/ci.yml": "modified",
    "scripts/pr-check-runner.py": "modified",
    "scripts/pr-gate-plan.py": "modified",
    "scripts/test_pr_gate_plan.py": "modified"
  },
  "file_hashes": {
    ".github/workflows/ci.yml": "89e1c640d8822c417423d1880eb12afca7aa77b97b78f4c7205193a9dab65762",
    "scripts/pr-check-runner.py": "8f8ec65653dff492268285358cb6e103c37e6226883f2fdd1129cf620e9f8646",
    "scripts/pr-gate-plan.py": "b5e7f5e049f7e10e18b727eb0b61bc04c049a3f2eb1bb8cb83d6eaba11f2b118",
    "scripts/test_pr_gate_plan.py": "2b72e3203a993cf7c3473bca0a0c99f8cb32a861670dee64eb8934becd5bc055"
  },
  "required_checks": [
    "pr-planner-tests",
    "ci-policy",
    "ci-executor-verifier-tests",
    "ci-actionlint"
  ]
}
```

## Unbound Core SQL retirement regression CI qualification

Owner-authorized exact CI source qualification; no independent review is claimed. This three-file source adds one mandatory executable regression/safety profile and an exact immutable binding for product source `938adee78ec2d10536f1834829facd8d9b1bec09`. It reuses the existing trusted executor and source/ancestry checks; missing, skipped and failed test families remain blocking. All144 planner/runner tests, existing nested utility checks, the six actual Go regression/safety families and workflow policy pass locally. Native acceptance of the corrected product remains pending. Only the authenticated target copy of this record authorizes CI; candidate-owned copies cannot qualify themselves.

```json
{
  "repository": "mcpdev80/baseharbor",
  "target_branch": "develop",
  "reviewed_head": "1b5acfeb17a538755e69545e9339bac894f86d66",
  "files": {
    "scripts/pr-gate-plan.py": "modified",
    "scripts/pr-check-runner.py": "modified",
    "scripts/test_pr_gate_plan.py": "modified"
  },
  "file_hashes": {
    "scripts/pr-gate-plan.py": "bb38f470b0fc7878d83d858e2a15e6211626777112bef21613e0936728ab9d6e",
    "scripts/pr-check-runner.py": "3a9ff8b808195ceb3dda8ae2d059f1fcafff429131f11d709f96485d94782ad9",
    "scripts/test_pr_gate_plan.py": "98dcd7df9453e7539da3ff568f59aa0f5752f22e68a6918f45cd7dc1425c7cc9"
  },
  "required_checks": [
    "pr-planner-tests",
    "ci-policy",
    "ci-executor-verifier-tests"
  ]
}
```
