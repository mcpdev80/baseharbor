# Targets and deployment destinations

BaseHarbor separates **where** an application is operated from **what** the application needs and **which environment** is selected.

A concrete deployment identity is:

```text
target + application + environment
```

Examples:

```text
local    / demo     / dev
k3s      / demo     / dev
k8s-prod / mailflow / prod
```

## Target model

A BaseHarbor target is deployment/operator state:

```text
Target
├── Runtime Provider
├── Runtime Access Reference
└── Target Scope
```

Representative targets:

```text
local
  runtime: docker
  access: local
  scope: default

k3s
  runtime: kubernetes
  access: homelab-k3s
  scope: default

k8s-prod
  runtime: kubernetes
  access: corp-prod
  scope: team-a-prod
```

K3s is represented as a Kubernetes target rather than a new portable runtime type. Kubernetes/OpenShift-specific access and scope interpretation remain behind their Runtime Provider boundaries.

Target, application and environment are independent axes. A target does not imply `dev`, `test` or `prod`, and an environment does not select a runtime.

## Repository versus installed BaseHarbor state

The repository remains authoritative for portable application intent:

```text
baseharbor.yaml
envs/<environment>/baseharbor.yaml
```

The current working directory may help BaseHarbor determine the current application and environment. It must never determine which deployments the installed BaseHarbor instance knows.

In short:

```text
cwd may answer "which application do I mean?"
cwd must not answer "which applications does BaseHarbor know?"
```

Target definitions are user configuration. Deployment/runtime state is user-global BaseHarbor state.

```text
$XDG_CONFIG_HOME/baseharbor/config.yaml
~/.config/baseharbor/config.yaml

$XDG_DATA_HOME/baseharbor/
~/.local/share/baseharbor/
```

## Target selection

The effective target resolves in this order:

```text
explicit --target
        ↓
activated BASEHARBOR_TARGET
        ↓
configured default target
        ↓
local
```

Application and environment resolution remain separate.

## Shell-local activation

BaseHarbor targets are designed for shell-local activation, similar to a Python virtual environment.

That means separate terminals can safely work against different targets at the same time:

```text
Terminal A -> local
Terminal B -> k3s
Terminal C -> k8s-prod
```

The active target is represented locally in the shell and wins over the configured default.

## Prompt visibility

BaseHarbor should make the active target visible **before a command is entered**.

Prompt integration is optional and configurable. The default presentation should remain compact, for example:

```text
[k3s] ~/projects/demo $
~/projects/demo [k3s] $
```

Environment-aware color may provide an additional signal:

- dev: soft green;
- test/stage: soft amber;
- prod: muted red.

Color is never the only production indicator. Accessible configurations can add explicit text such as `TEST` or `PROD`.

The prompt setup supports compact presets, live preview, text-only/accessibility modes and placement before the path, after the path, or as a right prompt where the shell supports it reliably.

## Listing deployments

Application discovery is target/global-state driven rather than repository-local.

A selected-target view can show:

```text
APPLICATION  ENVIRONMENT  STATE
demo         dev          READY
demo         test         STOPPED
mailflow     dev          READY
```

An all-target view can show:

```text
CONTEXT   APPLICATION  ENVIRONMENT  RUNTIME     STATE
local     demo         dev          docker      READY
k3s       demo         dev          kubernetes  READY
k8s-prod  mailflow     prod         kubernetes  READY
```

The same result must be available regardless of the current directory.

## Machine parity

Target/deployment identity is part of the shared semantic core. CLI, JSON and MCP expose the same effective target metadata. No interface may bypass target selection, ownership, policy or lifecycle verification.

This v0.4.15 foundation is tracked in issue #408 and is intentionally designed so the later Runtime Provider work in #396 and Kubernetes/OpenShift implementations can consume it without changing portable application intent.
