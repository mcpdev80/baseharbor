# BaseHarbor roadmap

BaseHarbor is a **secure application backend runtime** for independent applications.

It provisions, secures and operates isolated backend service stacks while allowing applications to use standard protocols and native client libraries. BaseHarbor is not intended to become a general-purpose application hosting platform or a proprietary backend API that applications must depend on.

## Product contract

BaseHarbor should make this workflow possible:

```bash
baha app create myapp
baha app apply myapp
baha app status myapp
baha app doctor myapp
```

After provisioning, an application should receive normal service endpoints and credentials such as:

```text
DATABASE_URL=postgresql://...
REDIS_URL=redis://...
OPENBAO_ADDR=https://...
S3_ENDPOINT=https://...
```

The application then uses the normal ecosystem for its programming language.

| Capability | Public integration boundary |
| --- | --- |
| Database | PostgreSQL protocol |
| Cache | Redis/Valkey protocol |
| Object storage | S3-compatible API |
| Authentication | OIDC/OAuth2 |
| Secrets | OpenBao/Vault-compatible API |
| Metrics | Prometheus/OpenMetrics |
| Telemetry | OpenTelemetry |
| BaseHarbor-specific operations | REST/OpenAPI |
| AI/agents | OpenAI-compatible APIs and MCP where useful |

**Principle:** BaseHarbor manages infrastructure and security without forcing applications into a proprietary SDK.

## Isolation model

Applications remain independent. BaseHarbor does not put unrelated applications into one shared application database.

The default target model is dedicated stateful service instances per application where isolation is valuable:

```text
BaseHarbor control plane
│
├── app: mailflow
│   ├── PostgreSQL
│   ├── Redis/Valkey
│   ├── OpenBao secrets runtime
│   └── object storage when enabled
│
└── app: example-api
    ├── PostgreSQL
    ├── Redis/Valkey
    └── OpenBao secrets runtime
```

Shared platform services are appropriate only where duplicating them adds little security or operational value, for example the BaseHarbor control plane and central observability pipeline.

A platform OpenBao instance may hold BaseHarbor bootstrap and management secrets. Application secrets must remain isolated by application, with dedicated app OpenBao runtimes or an equivalent strongly isolated policy model depending on the selected deployment profile.

## Non-goals

BaseHarbor should **not** become another Coolify, Dokploy or generic PaaS.

The project should not build its own replacements for mature infrastructure when standard open-source components already solve the problem.

Out of scope for the core product:

- Git repository deployment pipelines
- source builds and buildpacks
- general-purpose container hosting UI
- container registry implementation
- proprietary database protocol or ORM
- proprietary cache protocol
- proprietary object-storage API
- proprietary telemetry SDK
- a new secrets engine, SQL database, cache, ACME server or monitoring database

The value of BaseHarbor is the secure provisioning, integration and lifecycle of those components.

---

# Phase 0 — Foundation

**Status: in progress / partially implemented**

Build the reliable control-plane foundation before adding broad service coverage.

Current foundations include:

- `baha` CLI
- configuration handling
- `init`, `up`, `down`, `status` and `doctor` lifecycle work
- Docker/Podman Compose runtime abstraction
- PostgreSQL connection and migration layer
- tenant and authorization foundations
- PostgreSQL RLS foundations
- provider-neutral authentication contracts
- stable API error model
- OpenBao credential adapter
- dependency update automation
- first Compose runtime with PostgreSQL and OpenBao

### Exit criteria

- lifecycle commands fail truthfully instead of treating a running container as a healthy service
- generated local secrets are protected from accidental Git commits
- runtime configuration is deterministic and testable
- PostgreSQL runtime and migration identities are least-privilege and separated
- security-sensitive operations have regression tests

---

# Phase 1 — First-class application model

This is the first major product milestone and the point where BaseHarbor becomes more than a Compose wrapper.

Introduce a first-class `Application` resource and a declarative manifest.

Example:

```yaml
version: 1

app:
  name: myapp

environment: production

services:
  postgres:
    enabled: true
    storage: 20Gi

  redis:
    enabled: true

  secrets:
    enabled: true
```

Target CLI:

```bash
baha app create myapp
baha app apply myapp
baha app up myapp
baha app down myapp
baha app status myapp
baha app doctor myapp
baha app env myapp
baha app destroy myapp
```

### Required behavior

- isolated application network
- dedicated PostgreSQL instance and volume when requested
- dedicated Redis/Valkey instance and volume when requested
- isolated application secrets runtime
- generated credentials unique to the application
- idempotent reconciliation: repeated `apply` converges instead of duplicating resources
- no unrelated application can reach another application's services by default
- environment names such as `dev`, `test` and `prod`
- safe teardown with explicit protection against accidental data deletion

### Developer experience

A developer must not need to understand BaseHarbor internals after provisioning. Standard clients must work directly.

```bash
baha app env myapp --format dotenv
baha app env myapp --format json
baha app env myapp --format yaml
baha app env myapp --format shell
```

Sensitive values should be masked by default and revealed or injected only through an explicit operation.

### Exit criteria

A small demo application in at least Go, Python and JavaScript/TypeScript can use the same provisioned backend through native clients without a BaseHarbor SDK.

---

# Phase 2 — OpenBao bootstrap, credentials and trust plane

Make OpenBao the central security foundation instead of leaving durable credentials in local runtime files.

### Platform secrets

BaseHarbor must securely manage:

- bootstrap credentials
- application service credentials
- database credentials
- Redis/Valkey credentials
- object-storage credentials
- ACME account and DNS-provider credentials
- service identities
- certificate material where appropriate

`baha` may create credentials, but OpenBao becomes the authoritative store after provisioning.

### Secret classes

Support three explicit classes:

1. bootstrap secrets
2. static managed secrets
3. dynamic credentials

### Dynamic credentials

Where supported, prefer short-lived credentials over long-lived passwords.

Initial target:

- PostgreSQL dynamic credentials through OpenBao
- revocation and TTL handling
- rotation without application-wide manual credential replacement

Later targets may include other services where the security and operational model is mature enough.

### Bootstrap requirements

- no static development/root token in production profiles
- initialize/unseal workflows must be explicit and auditable
- unseal material must never be casually persisted next to application data
- support manual/unseal-key workflows first
- design for external KMS/HSM/auto-unseal later
- OpenBao references and URLs must be strictly validated and fail closed

### Exit criteria

A provisioned application can obtain its required credentials without BaseHarbor storing long-lived plaintext copies in normal local configuration files.

---

# Phase 3 — Certificates, PKI and TLS

TLS must be a first-class BaseHarbor capability rather than an afterthought.

Support three equal certificate sources:

1. public ACME certificates
2. internal OpenBao PKI certificates
3. Bring Your Own Certificate (BYOC)

## Public ACME

Target:

```bash
baha cert add api.example.com --issuer letsencrypt
```

Requirements:

- automatic renewal
- HTTP-01 where appropriate
- DNS-01 for wildcard and non-public-edge use cases
- provider credentials stored in OpenBao
- support additional ACME-compatible CAs later

## Internal PKI

Use OpenBao PKI for internal service certificates and eventually mTLS.

Target hierarchy:

```text
protected/offline root CA
        │
        ▼
BaseHarbor intermediate CA
        │
        ├── application certificates
        ├── service certificates
        └── mTLS service identities
```

Enterprise deployments must be able to use an externally signed intermediate instead of requiring a BaseHarbor-owned root.

## Bring Your Own Certificate

Reuse and improve the proven guided-certificate workflow:

```bash
baha cert import /path/to/certificate-folder
```

The user provides a directory, not a fragile list of individual files.

Initial supported inputs:

- PEM
- CRT
- CER
- KEY
- ZIP certificate bundles
- PKCS#7 (`.p7b`, `.p7c`)
- PKCS#12 (`.p12`, `.pfx`)

BaseHarbor should automatically:

1. discover candidate files
2. unpack supported bundles
3. normalize certificate encodings
4. identify the usable private key
5. find the leaf certificate by matching its public key to the private key
6. identify intermediates
7. build the certificate chain
8. verify hostname/SAN coverage
9. detect wildcard certificates
10. verify validity dates
11. verify the chain against configured trust roots
12. validate key usage and `serverAuth` EKU where applicable
13. activate the new certificate only after complete validation

Replacement must be atomic: a broken replacement must never remove a currently working certificate.

### Doctor integration

`baha doctor` and `baha app doctor` should report:

- hostname match
- chain validity
- certificate expiry
- issuer
- renewal configuration
- renewal health

### Exit criteria

The same application can use automatic ACME, internal PKI or imported enterprise/provider certificates without application-code changes.

---

# Phase 4 — Backup, restore and recovery

Provisioning is not enough. BaseHarbor must be able to recover the services it creates.

Target commands:

```bash
baha app backup myapp
baha app restore myapp
baha app backup verify myapp
```

Requirements:

- PostgreSQL-consistent backups
- application-aware service metadata
- encrypted backup destinations where appropriate
- S3-compatible backup target
- retention policy
- restore into the same or a new environment
- schema/runtime compatibility validation before resume
- backup verification instead of assuming that an uploaded file is restorable
- restore must fail closed when required secrets or keys are missing

Later:

- scheduled backups
- point-in-time recovery where supported
- environment clone from backup

### Exit criteria

A documented disaster-recovery test can destroy and recreate a demo application's backend from backup without reconstructing hidden manual state.

---

# Phase 5 — Observability and operations

Observability stays in scope because BaseHarbor operates the services it provisions.

BaseHarbor should integrate standards rather than create a proprietary telemetry system.

### Standards

- OpenTelemetry for traces/telemetry
- Prometheus/OpenMetrics for metrics
- structured stdout/stderr and OTLP-compatible log pipelines

Possible managed components include mature OSS such as Prometheus, Grafana, Loki and Tempo, but they remain replaceable implementation choices.

### Shared observability plane

Do not deploy an entire observability stack per application by default.

Instead, use one platform observability plane with application/service labels and access isolation.

Example dimensions:

```text
app=myapp
environment=production
service=postgres
instance=myapp-production
```

### BaseHarbor operational signals

At minimum monitor:

- service health
- PostgreSQL availability
- Redis/Valkey availability
- OpenBao initialized/sealed state
- certificate validity and renewal
- backup age and verification status
- disk capacity
- runtime/container failures
- BaseHarbor reconciliation failures
- metrics/log/telemetry pipeline health

Target:

```bash
baha app status myapp
baha app doctor myapp
```

must provide a useful operational answer without requiring Grafana for basic diagnosis.

### Exit criteria

A failed dependency, expired certificate, stale backup or unavailable database is visible through BaseHarbor health/doctor output and exported through standard observability interfaces.

---

# Phase 6 — Storage and application platform services

Add optional backend modules after the core runtime lifecycle is proven.

Initial candidates:

- S3-compatible object storage
- OIDC provider integration and optional bundled provider
- jobs/workflows
- realtime/event delivery
- webhooks/SSE
- audit/event history

Every module must follow the same rules:

- native/open protocol where one exists
- isolated application resources
- least-privilege credentials
- lifecycle owned by BaseHarbor
- health/doctor support
- backup/restore story before being considered production-ready

---

# Phase 7 — Stable BaseHarbor API and generated clients

BaseHarbor-specific operations need a stable API, but applications must not be forced to route PostgreSQL, Redis, S3 or OpenBao traffic through it.

### API contract

- `/api/v1/...`
- OpenAPI is the source of truth
- stable machine-readable errors
- provider-neutral authentication middleware
- explicit authorization and application boundaries
- backward-compatible evolution inside a major API version

### SDK policy

SDKs are optional thin generated clients for BaseHarbor-specific operations.

Initial generated targets may include:

- TypeScript
- Python
- Go
- Java
- C#

Raw HTTP remains a supported first-class integration path.

### Exit criteria

A developer in any language with an HTTP client can automate BaseHarbor without needing the `baha` process embedded in their application.

---

# Phase 8 — AI, MCP and RAG modules

AI capabilities remain optional platform modules, not requirements for the core runtime.

Targets:

- OpenAI-compatible inference integration
- external/local model gateway integration
- MCP server/client integration where useful
- RAG primitives
- model/service health
- credentials and policies through the same OpenBao trust model
- observability through the same platform pipeline

Do not turn BaseHarbor into a model-serving implementation when an external inference runtime or gateway already solves that problem well.

---

# Phase 9 — Multi-node and Kubernetes

Single-node Docker/Podman remains a first-class supported deployment, not a temporary demo mode.

Only after the application model and lifecycle are stable should BaseHarbor add broader orchestration targets.

Potential targets:

- remote Docker/Podman hosts
- multi-node placement
- high availability profiles
- Kubernetes deployment engine
- Kubernetes-native secret and identity integration
- external managed PostgreSQL/Redis/S3/OpenBao adapters

The application manifest and public integration contract should remain stable across deployment engines.

---

# MVP definition

The first meaningful BaseHarbor MVP is **not** the number of supported modules.

It is this complete vertical workflow:

```bash
baha app create demo
baha app apply demo
baha app status demo
baha app doctor demo
baha app env demo
baha app backup demo
```

with:

- isolated network
- dedicated PostgreSQL
- dedicated Redis/Valkey
- isolated OpenBao-backed secrets
- secure generated credentials
- working TLS
- ACME or BYOC certificate path
- health checks that test real service readiness
- basic metrics/log integration
- verified backup/restore path

A demo application must be able to consume the resulting services directly through native protocols.

Only after this vertical slice is reliable should the project add many optional modules.

# Release direction

## v0.x — prove the runtime

Focus on:

- application model
- declarative manifest
- isolated service provisioning
- OpenBao bootstrap and credentials
- certificate management
- backup/restore
- doctor/health
- observability baseline

Breaking changes are acceptable but should be deliberate and documented.

## v1.0 — stable application backend runtime

A v1.0 release should require:

- stable manifest schema
- stable CLI lifecycle semantics
- stable API v1
- upgrade compatibility guarantees
- tested backup/restore
- tested credential/certificate rotation
- supported production security profile
- documented support matrix
- migration path between supported BaseHarbor releases

# Decision rule for future features

Before adding a feature, ask:

1. Does this help provision, secure or operate an application's backend?
2. Can the application still use an open/native protocol?
3. Are we integrating a mature component instead of unnecessarily replacing it?
4. Can BaseHarbor health-check, upgrade, back up and recover what it creates?
5. Does this preserve application isolation?
6. Does it make the developer experience materially simpler?

If the answer is mostly no, the feature probably does not belong in BaseHarbor core.
