# Developer journey acceptance

BaseHarbor's developer promise is not only that individual commands work. A release must prove that a developer can start from an ordinary pristine repository, provide only information BaseHarbor cannot safely derive or generate, and reach a verified READY application without hidden manual state.

The external `mcpdev80/baseharbor-demo` repository is the canonical release-facing consumer for this journey.

## Canonical human journey

The product contract is:

```text
pristine ordinary repository
        |
        v
optional: baha app inspect .
        |
        v
baha app init
        |
        +--> detect workload and replaceable infrastructure
        +--> ask only about ambiguous/user-owned decisions
        +--> application-secret policy
        +--> human-readable adoption summary
        |
        v
baseharbor.yaml
        |
        v
baha up
        |
        +--> OpenBao first-run recovery handling
        +--> managed provider/runtime credentials
        +--> inline required application-secret input
        +--> safe port handling
        +--> backend/runtime convergence
        +--> workload build/start
        +--> readiness verification
        |
        v
READY
```

The normal journey must not require:

- manual YAML append/edit steps to complete detected intent;
- repository Compose rewriting;
- a separate OpenBao bootstrap command;
- a mandatory `plan -> preflight -> apply` command sequence;
- shell piping for normal human secret input.

`plan`, `preflight`, explicit `app apply`, `secret set --stdin` and lower-level lifecycle commands remain supported automation/troubleshooting interfaces.

## Guided and deterministic acceptance are separate

The demo acceptance suite intentionally contains two different proof paths.

### Guided human acceptance

The `guided` gate starts with:

```text
compose.yaml
application source
Dockerfiles

NO baseharbor.yaml
NO .baseharbor/
```

It drives the CLI through a real pseudo-terminal so terminal detection, prompts and hidden secret input use the same code paths as a person.

It proves:

- guided Compose selection when the repository contains multiple candidates;
- capability-first adoption;
- application-secret naming/policy;
- human-readable confirmation;
- unchanged repository Compose;
- OpenBao first-run handling through `baha up`;
- inline missing-secret resolution;
- convergence to READY in the same normal repository flow.

### Deterministic automation acceptance

The separate `init` gate exercises `baha app inspect` and `baha app init --quick` on an unambiguous ordinary repository fixture.

It proves that CI/agents can use deterministic detection without interactive assumptions and that heuristic secret candidates are not silently promoted.

Focused component gates may continue to use explicit low-level commands where that is the contract under test.

## Runtime parity

The release-facing demo acceptance suite runs against both supported local runtime implementations:

- Docker / Docker Compose;
- Podman / native Quadlet + rootless `systemd --user`.

Equivalent user-visible behavior must remain BaseHarbor-level behavior rather than provider-specific UX.

## Failure and recovery expectations

Known failures should be classified and actionable.

Examples:

- configurable occupied ports are resolved before workload start where safely possible;
- explicit operator port choices are never silently replaced;
- fixed occupied ports fail before workload start with a concrete action;
- missing required application secrets are resolved inline for an interactive human flow and fail closed with explicit automation remediation in non-interactive mode;
- raw Docker/Podman/provider output is diagnostic detail, not the primary human error.

## Release rule

The mandatory pre-release validation is the proof point for the complete developer journey.

Pre-release must pin and record both:

- the exact BaseHarbor candidate SHA;
- the exact `baseharbor-demo` SHA used as the external consumer contract.

Both complete journeys start immediately after source validation and candidate construction, alongside four light-gate lanes per runtime. Heavy gates use two lanes and start after their own runtime light matrix completes, without waiting for the other runtime or the independent reference journeys. HA groups subsequently use two lanes per runtime after their own heavy matrix completes. A reference journey can overlap those lanes, so these runtime matrices and journeys can use up to five Docker and five Podman jobs concurrently. Each job uses an isolated GitHub-hosted runner; the integration, static and adoption jobs have separate scheduling. A failed earlier phase does not suppress later independent gates.

The targeted branch `runtime-validation/targeted/batch/remaining/<suffix>` runs the remaining native atomic diagnostics in one workflow with eight concurrent jobs. It derives its gate inventory from the current release requirements, interleaves Docker and Podman, and excludes the already checked guided gates. It does not grant release approval or replace the complete final evidence set.

Static contract/DX gates run without provider containers. Docker and Podman gates run independently with the smallest declared resource profile for the behavior under test, including dedicated provider-semantic gates for durable key-value, MongoDB document storage and RabbitMQ queue/pubsub/stream semantics. A failed gate is debugged and repeated independently; successful unrelated gates are not rerun during diagnosis.

Use GitHub Actions **Re-run failed jobs** for the same unchanged candidate. An evidence-only rerun may use successful gate artifacts from earlier attempts. The selector always uses the newest attempt of each logical gate, including journeys; a new failure cannot be hidden by an older success. Selected manifests must match the candidate, demo pin, workflow run, runtime/gate identity, artifact attempt and successful cleanup. Superseded artifacts remain available separately. Any source fix creates a new candidate and requires its own complete evidence.

Failure summaries retain bounded, redacted original assertions before outer cleanup. Final evidence records per-job duration, retries, repeated failure classes and the longest jobs. Queue time is reported only when Actions supplies an actual job creation timestamp; it is otherwise unknown.

The release-boundary approval is valid only when the complete expected atomic gate set is reproduced on one unchanged BaseHarbor candidate SHA and one pinned external demo SHA, every required gate reports success, and the immutable evidence manifest is complete. The final release workflow consumes that approval and must not rerun the same expensive acceptance work.

## Product rule

> Clone an ordinary application repository, run `baha app init`, then `baha up`. Provide only what BaseHarbor cannot safely derive or generate, and get a verified working application.

Once `baseharbor.yaml` exists, `baha up` remains the normal lifecycle command.
