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


## Input-equivalent release evidence reuse CI qualification

Owner-authorized qualification of exact source; no independent-review claim. This three-file CI source binds only the two-file collector/regression candidate `c7e629c2876772493edc037769f1643c45c92fe8` to the existing planner, workflow-policy and executable verifier suites. The existing real-Git authority mutation matrix covers clean target integration and rejects missing/extra/status-altered files, changed anchors, unreviewed followups and injected merge trees. The runner docstring is clarified without execution changes. Collector validation passes104 targeted checks, with four new regressions failing against original source. Source-only65-gate input comparison remains equal across the collector-only change; no new execution or release approval is claimed. Original ZIP/source/attempt, cleanup, current consumer bindings and newer negative outcomes remain required.

```json
{
  "repository": "mcpdev80/baseharbor",
  "target_branch": "develop",
  "reviewed_head": "ac792db81cfeb4e0d6aa2570bf12f0eb48ae92d6",
  "files": {
    "scripts/pr-gate-plan.py": "modified",
    "scripts/pr-check-runner.py": "modified",
    "scripts/test_pr_gate_plan.py": "modified"
  },
  "file_hashes": {
    "scripts/pr-gate-plan.py": "0c0a604c1af5e2eb579655fb55970de1b36c4dfb361f412592e703775176e221",
    "scripts/pr-check-runner.py": "91a3078c34f848a434bba4525670127e7cc8c012fa43cd935edd22aea0c61595",
    "scripts/test_pr_gate_plan.py": "5f07dbf0bb008713e7dfc7f3d2e039adf4fc18ba190759be0c4cb6393697ed0c"
  },
  "required_checks": [
    "pr-planner-tests",
    "ci-policy",
    "ci-executor-verifier-tests"
  ]
}
```


## SQL Edit gate selection

Owner-authorized exact public CI source; no independent-review claim. Select the two SQL New/Edit gates and require the preview-error regression. Local127 planner checks and existing8/7/5 utilities pass. No gate waiver or native-success claim.

```json
{
  "repository": "mcpdev80/baseharbor",
  "target_branch": "develop",
  "reviewed_head": "114a3d70dc51a48a661590dbee34d275d946c490",
  "files": {
    "scripts/pr-gate-plan.py": "modified",
    "scripts/pr-check-runner.py": "modified",
    "scripts/test_pr_gate_plan.py": "modified"
  },
  "file_hashes": {
    "scripts/pr-gate-plan.py": "ce7c48675f9f3fec50ca2664c0424891204ffe1b6d6fb3f72d16bce05b31d228",
    "scripts/pr-check-runner.py": "600bec6402b907f6ad4d00a05178bdc6929d99363b794bf2f9084704d60ed492",
    "scripts/test_pr_gate_plan.py": "dc001ae1854ef85670e71ac4d7fe2f431d1ac7b655c360e77840f9bc7ecdeb23"
  },
  "required_checks": [
    "pr-planner-tests",
    "ci-policy",
    "ci-executor-verifier-tests"
  ]
}
```


## Corrected SQL Edit selection qualification

Owner-authorized continuation of the explicitly approved public CI register update. Supersedes the earlier SQL Edit candidate, whose redundant extra test-family entry failed the existing runner fixture. The existing TestAppEditCommand family runs the preview regression without an extra overlapping selector. Full145 planner/runner tests and nested8/7/5 utility checks pass locally. Both real SQL gates and required CI remain mandatory; no native success, gate waiver or independent review claimed.

```json
{
  "repository": "mcpdev80/baseharbor",
  "target_branch": "develop",
  "reviewed_head": "555f07d529c8389c8fccdd1089cf6c54bb97873f",
  "files": {
    "scripts/pr-gate-plan.py": "modified",
    "scripts/pr-check-runner.py": "modified",
    "scripts/test_pr_gate_plan.py": "modified"
  },
  "file_hashes": {
    "scripts/pr-gate-plan.py": "ce7c48675f9f3fec50ca2664c0424891204ffe1b6d6fb3f72d16bce05b31d228",
    "scripts/pr-check-runner.py": "262e6e079d6bdaa7f67734cfc758718953bcac2ced3004c4038605e81459d165",
    "scripts/test_pr_gate_plan.py": "dc001ae1854ef85670e71ac4d7fe2f431d1ac7b655c360e77840f9bc7ecdeb23"
  },
  "required_checks": [
    "pr-planner-tests",
    "ci-policy",
    "ci-executor-verifier-tests"
  ]
}
```

## Exact SQL remove and replacement preview qualification

Owner-authorized continuation of the approved public CI register publication; no independent-review claim. This exact CI source binds only product `57f5d268835a07f4b66607a29a8f4f2331f19b36` to mandatory Go/Guided/Edit/provider-choice checks and both real SQL gates. Regressions reproduce inherited removed preferences, a bound-selection refusal and a replacement binding-conflict. Corrected detached projections preserve persisted choices, the actual registry and other environments; execution remains under existing consequence consent. Full146 planner/runner tests, nested8/7/5 utilities and19 workflow-policy checks pass locally. Real Git tests reject incomplete/status-altered inventories, changed anchors, unreviewed followups and injected merge trees. Native failures remain recorded; no native success or gate waiver is claimed.

```json
{
  "repository": "mcpdev80/baseharbor",
  "target_branch": "develop",
  "reviewed_head": "15af842967511591616c1504ac841f05a099519b",
  "files": {
    "scripts/pr-gate-plan.py": "modified",
    "scripts/pr-check-runner.py": "modified",
    "scripts/test_pr_gate_plan.py": "modified"
  },
  "file_hashes": {
    "scripts/pr-gate-plan.py": "2b31755edac73f159e1a06f461e410298abc10fa628f2673a4f4604bef002aad",
    "scripts/pr-check-runner.py": "03d1b38b2f8e6a0c2ec6c783e42bc1acc23aaff7eb275a44c1920528734541ef",
    "scripts/test_pr_gate_plan.py": "008eab8dfa214e52e9c4c881f28997d273d8af065004a15cce9649f8304703e7"
  },
  "required_checks": [
    "pr-planner-tests",
    "ci-policy",
    "ci-executor-verifier-tests"
  ]
}
```
