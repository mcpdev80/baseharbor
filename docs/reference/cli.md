# CLI reference

The `baha` CLI is BaseHarbor's primary human interface.

Use `baha --help` and command-specific `--help` as the authoritative syntax source.

Common lifecycle:

| Command | Purpose | Mutates |
|---|---|---:|
| `baha app inspect` | inspect repository evidence | no |
| `baha app init` | create/adopt application contract | repository |
| `baha plan` | resolve intended changes | no |
| `baha up` | reconcile desired runtime state | yes |
| `baha status` | show current semantic state | no |
| `baha doctor` | diagnose readiness/problems | no |
| `baha doctor --fix` | apply bounded safe repairs | yes |
| `baha backup` | create and verify supported backup | yes |
| `baha destroy` | remove BaseHarbor-owned resources | yes |

Machine-readable output uses the same semantic core:

```bash
baha plan -o json
baha status -o json
baha doctor -o json
```

Exact command flags are generated from the implementation and must not be duplicated manually here when `--help` is sufficient.
