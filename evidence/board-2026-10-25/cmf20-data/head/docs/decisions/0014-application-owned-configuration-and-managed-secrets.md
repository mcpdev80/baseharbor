# ADR 0002: Application-owned configuration and BaseHarbor-managed secrets

- Status: Accepted
- Date: 2026-09-08

## Context

BaseHarbor's first reference applications are Agent Workflow Coordinator (AWC), MailFlow and AI-Coding-System (ACS). All three use AI/LLM providers, but provider selection and configuration belong to the applications themselves:

- AWC currently receives its Agent Gateway configuration primarily through runtime configuration/environment variables.
- MailFlow lets users configure AI providers and endpoints at application runtime.
- ACS is designed to expose provider configuration through its own application/provider model and UI.

If BaseHarbor also became the authoritative store for provider selection, endpoint, model or similar application-domain configuration, the same configuration would exist in two places and BaseHarbor would take ownership of application business logic.

At the same time, provider credentials such as API keys should not need to be stored as plaintext in normal application configuration or databases when BaseHarbor is available.

BaseHarbor must also preserve its no-lock-in property: the same application must remain usable without BaseHarbor.

## Decision

### 1. Application configuration stays application-owned

Configuration that belongs to an application's domain remains authoritative in that application.

Examples include:

- provider selection
- provider endpoint/base URL
- model selection
- routing rules
- provider-specific options

BaseHarbor does not duplicate or override that configuration unless a future capability is explicitly designed as an optional integration.

### 2. Sensitive credentials may be BaseHarbor-managed

Applications may store sensitive credentials through BaseHarbor's managed secret plane backed by OpenBao.

The application stores only a stable logical secret reference/handle with its own configuration, for example conceptually:

```text
provider_id = agentgateway-main
endpoint = http://192.168.6.132:4000
model = qwen
credential_ref = baseharbor://secrets/providers/agentgateway-main/api-key
```

The real credential value is stored in OpenBao and is never required in the application's normal configuration database.

The exact URI/handle syntax is not fixed by this ADR. The public contract must remain small, stable and suitable for use by independent applications.

### 3. Required startup secrets remain supported

Static/runtime credentials that are known at deployment time continue to use the existing required-secret contract, for example:

```yaml
secrets:
  required:
    - name: AGENT_GATEWAY_API_KEY
```

BaseHarbor resolves and injects those values into the application's runtime using the normal runtime contract.

### 4. Dynamic application secrets are an MVP capability

The MVP must support application-created secrets whose names/identities are not known when `baseharbor.yaml` is written.

This is required for applications such as MailFlow and ACS where users may add provider credentials later through the application UI.

A dynamic secret must:

- be scoped to exactly one BaseHarbor application/environment
- be stored in OpenBao or the selected BaseHarbor secret provider
- be retrievable only by the owning application/runtime identity and authorized operators
- support create, read/use, update/rotation and delete lifecycle
- never leak through `status`, `doctor`, logs, API errors or normal CLI output
- have a stable reference that the application can persist alongside its own configuration
- fail closed when the referenced value is missing or unreadable

### 5. Standalone applications remain first-class

BaseHarbor-managed secrets are optional infrastructure convenience, not a mandatory runtime dependency.

An application must remain able to operate without BaseHarbor by using its own supported secret mechanism, environment variables, encrypted database storage or another credential provider.

Applications must not require:

- `baha login`
- a BaseHarbor runtime token solely to use their normal application functionality
- a mandatory BaseHarbor SDK
- a proprietary BaseHarbor protocol for PostgreSQL, Redis/Valkey, HTTP, OIDC or AI provider traffic

### 6. BaseHarbor does not become an AI provider registry for the MVP

AI/LLM use does not by itself create a `services.ai` requirement in `baseharbor.yaml`.

For the MVP, BaseHarbor is responsible for:

- secure credential storage when requested
- runtime secret delivery
- application network connectivity/egress
- health/diagnostic reporting for BaseHarbor-owned infrastructure

The application remains responsible for deciding which AI provider, endpoint and model it uses.

## Reference application mapping

### AWC

AWC may declare known runtime credentials such as `AGENT_GATEWAY_API_KEY` as required BaseHarbor secrets. Endpoint/provider behavior remains AWC configuration.

### MailFlow

MailFlow owns provider, endpoint and model configuration. Provider API keys may be stored as dynamically created BaseHarbor-managed secrets and referenced from MailFlow's configuration records.

### AI-Coding-System

ACS owns its provider model and future provider UI. Provider credentials may use the same dynamic BaseHarbor-managed secret contract while ACS stores only the corresponding secret reference.

## Consequences

### Positive

- application and BaseHarbor ownership boundaries remain clear
- no duplicate source of truth for provider configuration
- sensitive credentials can be removed from ordinary application databases
- the same application can run with or without BaseHarbor
- the model works for AI providers and for other dynamically configured credentials such as SMTP, webhooks or external APIs

### Trade-offs

- applications that want managed dynamic secrets need a small integration boundary for secret references
- BaseHarbor must provide a secure runtime/API mechanism for dynamic secret operations
- secret-reference compatibility and migration semantics must be defined before v1.0

## MVP acceptance implications

The BaseHarbor MVP is not complete until at least one reference application proves both secret paths:

1. a deployment-time required secret is injected successfully; and
2. a credential created by the application at runtime can be stored in BaseHarbor/OpenBao, referenced by the application, rotated and deleted without exposing its plaintext value.

MailFlow is the preferred reference application for the second path because provider configuration is already user-managed at runtime.
