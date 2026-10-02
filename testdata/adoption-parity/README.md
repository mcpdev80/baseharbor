# Cross-source reference application

This reference application proves that source syntax does not define portable BaseHarbor semantics.

The same logical application is represented through three independent source families:

- Compose
- repository-authored Podman Quadlet
- raw Kubernetes YAML

Expected common semantics:

```text
workload components
  api
  worker

embedded infrastructure
  db       -> database.sql
  cache    -> cache.key-value

api
  exposed on port 8080
  health/readiness evidence present

all components
  source provenance present

source
  deterministic fingerprint present
```

The executable acceptance is `TestWorkloadSourceCrossSourceSemanticParity`.

Only semantics genuinely representable by all three source formats belong in this common matrix. Source-native names and paths are provenance and are intentionally not expected to match.
