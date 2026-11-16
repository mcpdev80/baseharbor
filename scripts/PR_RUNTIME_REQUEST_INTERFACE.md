# PR runtime request interface

`scripts/pr-runtime-request.py` selects existing Docker/Podman gates from the
trusted plan and release requirements. Every request contains the exact target
base, head, merge base, integration commit and required consumer source pins.
It contains no test result, receipt or evidence reference.

`scripts/pr-runner-isolation.py` admits only the matching GitHub-hosted,
credential-free PR event from GitHub's local event metadata. Repository identity,
Owner PR, branch, source commits and run/attempt must match. Forks and self-hosted
runners cannot enter this execution path. No Actions API witness is queried.

Each PR/run/attempt/gate has its own disposable target and workspace, namespace,
ports, volumes, networks, provider, secret and recovery state. Atomic capacity
limits never evict peers. Cleanup retains the lease until all its owned runtime
resources are absent; foreign resources remain untouched.

CI waits for the normal selected jobs through `needs` and checks their direct
results. Failed, missing, skipped, cancelled or timed-out required jobs block the
final check. Binary/image inputs must belong to the exact integration source.
There are no gate manifests, artifact result verification or retained results.
