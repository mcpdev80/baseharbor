# Developer journey acceptance

BaseHarbor's developer-facing promise is broader than individual command correctness. A release should prove that a developer can start from a clean application checkout, provide only information BaseHarbor cannot safely derive, and reach a healthy application without hidden manual state.

The `developer-journey` GitHub Actions workflow is the release-facing acceptance layer for that promise.

## What the journey proves

The first reference consumer is MailFlow from its `main` branch.

The workflow validates this sequence:

```text
clean BaseHarbor checkout
        |
        v
build exact baha + runtime image under test
        |
        v
occupy common ports 5432 / 8200
        |
        v
clone MailFlow main
        |
        v
baha up --yes --recovery-file <secure-path>
        |
        +--> safe control-plane port fallback
        +--> OpenBao bootstrap
        +--> repository detection
        +--> required-secret failure is visible and fail-closed
        |
        v
supply only SECRET_KEY securely via stdin
        |
        v
baha up --yes
        |
        +--> reuse control plane + OpenBao
        +--> converge managed backends
        +--> start MailFlow workload
        +--> verify readiness
        |
        v
status + doctor
        |
        v
verify real MailFlow workload and native backend URLs
        |
        v
app down + baha up --yes + doctor
        |
        v
READY
```

The normal repository happy path is therefore `baha up`. Advanced commands such as `baha app plan`, `baha app apply` and `baha app doctor` remain available for CI, automation and troubleshooting, but they are not required knowledge for the basic developer path.

## Why this is separate from unit and integration tests

Unit and package tests remain the fast correctness layer. Existing runtime, broker, backup/restore and application integration workflows test focused contracts in depth.

The developer journey answers a different question:

> Does the product still feel like one coherent path when a real developer uses it?

A change can pass isolated tests while accidentally requiring an undocumented command, relying on repository-local hidden state, producing an unusable missing-secret error, or breaking the restart path. The journey gate exists to catch those regressions.

## Required behavior

The workflow intentionally creates port conflicts on the default PostgreSQL and OpenBao ports. `baha up --yes` must select safe alternatives without modifying or destroying the unrelated containers occupying those ports.

On a fresh managed-secret installation, BaseHarbor cannot safely invent where the operator wants OpenBao recovery material stored. Interactive use asks for that path. Non-interactive use supplies `--recovery-file PATH`; the path remains outside BaseHarbor application state.

A required external application secret is intentionally absent during the first `baha up`. BaseHarbor must:

- start/reuse only the infrastructure needed to reach the readiness decision;
- refuse workload startup;
- identify the missing secret by name;
- keep the workload stopped;
- allow the developer to provide the value securely;
- continue successfully on the next `baha up` without manual state repair.

After successful startup, the workflow verifies that MailFlow receives standard application-facing interfaces such as `DATABASE_URL` and `REDIS_URL`. The application does not need a BaseHarbor SDK or runtime login.

The restart portion validates that `baha app down` preserves durable state and that the recommended repository command `baha up --yes` returns the application to a healthy state.

Focused operator or CI workflows that intentionally need only the BaseHarbor platform can use the explicit advanced mode:

```bash
baha up --control-plane-only
```

That flag is not the normal developer path.

## Relationship to the MVP reference matrix

This workflow establishes the reusable developer-journey pattern for issue #80 and the one-command application lifecycle from #81. MailFlow is the first consumer because it already exercises PostgreSQL, Valkey, managed secrets and a multi-service workload.

AWC and AI-Coding-System remain tracked in the final reference-app matrix (#45). Their acceptance should reuse the same principles instead of creating different operator workflows for each application.

## Release rule

Before the next stable release, the developer-facing happy path must be green together with the existing focused acceptance workflows.

The intended product rule is:

> Clone the application, run `baha up`, provide only what cannot be derived or generated safely, and get a verified working application.
