# Adoption Conformance Corpus

This corpus proves repository-adoption correctness separately from the real-world robustness corpus.

Validation layers:

1. `source-adapter-unit` — focused positive/negative unit tests.
2. `source-conformance` — controlled fixtures with known ground truth.
3. `source-cross-parity` — one reference application expressed through Compose, Quadlet and Kubernetes YAML.
4. `source-realworld` — pinned 10+10+10 public repositories covering real repository diversity.

Every controlled fixture owns an `expected.yaml` file. The assertions are semantic rather than source-format-specific.

Required outcome fields are intentionally small:

```yaml
resolution:
  state: selected
  reason: single_candidate
source:
  kind: compose
  path: compose.yaml
components:
  api:
    infrastructure: ""
  db:
    infrastructure: database.sql
```

Broken-input fixtures specify the expected fail-closed result instead of pretending malformed sources normalize successfully.

Real-world cases live in `testdata/adoption-realworld/corpus.yaml`; they record the pinned repository, revision, subpath, category tags and the repository pattern each case exists to prove.
