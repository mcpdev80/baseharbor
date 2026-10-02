# Workload sources

BaseHarbor separates repository workload syntax from portable Application Intent and Runtime Provider realization.

```text
Repository
  -> Workload Source Adapter
  -> Normalized Workload Evidence
  -> repository inspection / adoption
  -> portable Application Intent
  -> Runtime Provider
```

These are different concerns:

```text
Workload Source != Runtime Provider
Workload Source != Delivery Provider
Workload Source != Development Adapter
Workload Source != Portable Application Intent
```

## v0.4.20 source support

| Source | Inspect/adopt | Runtime realization in v0.4.20 |
| --- | --- | --- |
| Compose | yes | Docker/Podman existing repository-workload path |
| Podman Quadlet | yes | not yet for repository-authored Quadlet |
| Raw Kubernetes YAML | yes | not yet; Kubernetes Runtime comes later |
| Helm | deferred | deferred |
| Kustomize | deferred | deferred |

A repository can therefore be understood and adopted even when the selected current Runtime Provider cannot realize that source yet. Plan/preflight fails with a typed actionable unsupported result rather than making repository inspection fail.

## Standardized resolution outcome

Every repository scan emits a versioned source-resolution outcome in addition to candidate/evidence data.

```json
{
  "schema_version": "baseharbor.workload-source-resolution/v1",
  "state": "selected",
  "reason": "production_candidate_dominates",
  "candidate_count": 3,
  "selected": {
    "kind": "compose",
    "path": "docker/docker-compose.yml"
  }
}
```

Stable states are:

- `selected`
- `ambiguous`
- `not_detected`
- `invalid`
- `unsupported`

Stable reasons include:

- `single_candidate`
- `explicit_repository_selection`
- `production_candidate_dominates`
- `multiple_viable_candidates`
- `cross_family_ambiguity`
- `only_low_confidence_candidates`
- `no_supported_source`
- `invalid_repository_metadata`
- `unsupported_repository_metadata`

CLI, JSON and MCP consume the same resolution model. Consumers must not infer ambiguity solely from a missing selected source.

## Support and limitations

| Source | v0.4.20 inspection behavior | Important limits |
| --- | --- | --- |
| Compose | Detects and normalizes selected Compose sources, in-file YAML merge keys and in-file `extends`; deterministic `${VAR:-default}` / `${VAR-default}` defaults are resolved | top-level `include` and external `extends.file` fail explicitly; external `${VAR}` values are preserved as unresolved source evidence and are never read from the operator shell |
| Podman Quadlet | Detects repository-authored `.container`, `.pod`, `.network`, `.volume` and `.kube` evidence; local `.kube` YAML is normalized through the Kubernetes adapter | missing local material inputs fail closed; BaseHarbor does not rewrite repository units |
| Raw Kubernetes YAML | Static multi-document inspection of standard workload/supporting objects; unknown resources remain opaque evidence | no live-cluster lookup, no namespace ownership inference, bounded document count; Kubernetes Runtime realization is not part of v0.4.20 |
| Helm | not implemented | later Workload Source Adapter |
| Kustomize | not implemented | later Workload Source Adapter |

Repository collection is bounded by per-file size, total relevant bytes, relevant-file count and traversal depth. Symlinked repository files are skipped so repository-controlled links cannot escape the inspected tree.

## Normalized workload evidence

The source-neutral evidence model contains logical workload components and source provenance, including where representable:

- image/build evidence;
- ports/endpoints;
- health/readiness;
- environment/config references;
- dependencies;
- persistent storage;
- embedded infrastructure classification;
- exposure evidence;
- deterministic source fingerprint.

Unknown Kubernetes objects remain visible as opaque source evidence. Secret values are never projected into normal evidence.

## Source selection

If exactly one source is authoritative, no additional metadata is required.

If multiple viable source families are present, BaseHarbor does not silently choose based on product preference. Guided adoption asks once and persists the selection in:

```text
baseharbor.repository.yaml
```

Example:

```yaml
version: 1
workload-source:
  kind: kubernetes
  path: deploy/k8s
```

This file is safe-to-commit repository-authoring metadata. It is not portable Application Intent and it is not protected runtime/Target state.

## Adoption conformance

Scanner validation is intentionally split into four evidence layers:

```text
source-adapter-unit
  -> focused positive and negative adapter tests

source-conformance
  -> controlled fixtures with known semantic ground truth
  -> broken/adversarial inputs
  -> fail-closed security bounds

source-cross-parity
  -> one reference application represented as Compose, Quadlet and Kubernetes YAML
  -> equivalent normalized semantics where representable

source-realworld
  -> 10 Compose + 10 Quadlet + 10 Kubernetes public repositories
  -> pinned revisions + documented quality/pattern tags
```

The real-world corpus proves robustness against repository diversity. The conformance corpus proves correctness against known expectations. Neither replaces the other.

Controlled conformance cases cover source ambiguity, noisy candidate selection, Compose include/extends boundaries, Quadlet `.kube` references, Kubernetes multi-document/CRD/Secret/Job/CronJob behavior, invalid metadata, malformed YAML, symlink escape resistance and bounded scan inputs.

The canonical metadata lives under:

```text
testdata/adoption-conformance/
testdata/adoption-realworld/
```

Run the layers independently with:

```text
scripts/adoption-conformance.sh unit
scripts/adoption-conformance.sh conformance
scripts/adoption-conformance.sh parity
scripts/adoption-conformance.sh realworld
```

## Real-world regression corpus

The scanner is validated against pinned public repositories in `scripts/realworld-workload-source-corpus.sh`.

The release corpus contains 30 pinned real-world examples:

- 10 Compose examples;
- 10 repository-authored Podman Quadlet examples;
- 10 raw Kubernetes YAML examples.

The set intentionally spans reference-quality projects, normal production repositories, multi-service applications, large manifest directories, Homelab-style Quadlet repositories and mixed-source layouts.

A separate mixed Compose + Quadlet check uses ViTransfer to prove that BaseHarbor reports ambiguity instead of silently preferring one source family.

The corpus includes projects such as Appwrite, Immich, Supabase, Nextcloud, Grafana Loki, Open WebUI, Google microservices-demo, Bank of Anthos, Argo CD, kube-prometheus, ingress-nginx, Rook and several independent Quadlet repositories.

Real-world scanner failures become focused regression tests before detection heuristics are broadened. The Appwrite corpus case, for example, forced support for same-file Compose `extends` rather than accepting a curated-only detector.


## Compose interpretation boundary

Compose is interpreted at two different architectural layers for two different purposes:

- repository inspection/adoption resolves read-only source evidence and normalizes it into source-neutral Workload Evidence;
- the Podman Runtime Provider consumes the already selected repository workload and realizes the supported runtime subset as native Quadlet units.

The CLI/init layer does not maintain its own Compose parser. Runtime realization must not make repository-adoption or portable-intent decisions, and repository inspection must not own Podman runtime rendering.

