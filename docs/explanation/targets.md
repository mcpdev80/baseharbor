# Targets and deployment destinations

BaseHarbor separates **where** an application is operated from **what** the application needs and **which environment** is selected.

A concrete deployment identity is:

```text
target + application + environment
```

Examples:

```text
docker-dev / demo     / dev
podman-dev / demo     / dev
laptop-k3s / demo     / dev
prod-ocp   / mailflow / prod
```

## The target model

A BaseHarbor Target is a named deployment destination.

```text
Target
├── Runtime Provider
├── Runtime Access Reference
└── Target Scope
```

The concepts stay separate:

```text
Target
  where BaseHarbor operates

Runtime Provider
  which runtime implementation realizes workloads

Access
  how BaseHarbor reaches/authenticates to that runtime

Scope
  which logical area inside that runtime is selected
```

Target names carry no runtime semantics. A Target is not synonymous with local, remote, Docker, Podman, Kubernetes or OpenShift.

K3s, k3d, kind, minikube, MicroK8s and similar distributions are Kubernetes Targets rather than separate BaseHarbor Runtime Providers.

Representative examples:

```text
Target              Provider      Access          Scope
------------------------------------------------------------
docker-dev          docker        local-docker    default
podman-dev          podman        local-podman    default
laptop-k3s          kubernetes    laptop-k3s      dev
homelab-k3s         kubernetes    homelab         dev
homelab-k3s-test    kubernetes    homelab         test
customer-prod       openshift     customer-a      project-x
```

Multiple Targets may share one Access definition and select different Scopes.

## Repository versus installed BaseHarbor state

The repository remains authoritative for portable application intent:

```text
baseharbor.yaml
envs/<environment>/baseharbor.yaml
```

The current working directory may help identify the current application and environment, but it never defines which deployments the installed BaseHarbor instance knows.

```text
cwd may answer "which application do I mean?"
cwd must not answer "which applications does BaseHarbor know?"
```

## Config versus Target state

User configuration is global:

```text
$XDG_CONFIG_HOME/baseharbor/config.yaml
~/.config/baseharbor/config.yaml
```

It contains Target definitions, Access definitions, defaults and prompt preferences.

Mutable runtime/deployment state is Target-scoped:

```text
$XDG_DATA_HOME/baseharbor/targets/<target>/
~/.local/share/baseharbor/targets/<target>/
```

Conceptually:

```text
~/.local/share/baseharbor/
└── targets/
    ├── docker-dev/
    │   ├── runtime/
    │   ├── provider-registry.json
    │   └── deployments/
    ├── podman-dev/
    │   ├── runtime/
    │   ├── provider-registry.json
    │   └── deployments/
    └── laptop-k3s/
        ├── runtime/
        ├── provider-registry.json
        └── deployments/
```

This makes independent parallel Targets possible without sharing mutable BaseHarbor runtime state accidentally.

A local Kubernetes/K3s Target is not special: it is simply a Kubernetes Target whose Access definition reaches a local cluster.

## Target selection

The effective Target resolves in this order:

```text
explicit --target
        ↓
activated BASEHARBOR_TARGET
        ↓
configured default target
        ↓
first-run/local target selection
```

Application and environment resolution stay independent.

## Shell-local activation and prompt visibility

Targets are designed for shell-local activation, similar to Python virtual environments. Different terminals can therefore safely target different destinations at the same time.

The optional prompt segment keeps the selected Target visible before a BaseHarbor command is entered:

```text
[homelab] ~/projects/demo $
[prod-ocp PROD] ~/projects/mailflow $
```

Prompt style is configurable: compact or detailed, text-only or color-assisted, before/after the path, or right-prompt where supported.

Environment-aware colors may provide an additional signal:

- dev: soft green;
- test/stage: soft amber;
- prod: muted red.

Production must never rely on color alone.

## Target lifecycle and safety

Changing the active/default Target never moves or relabels existing deployments.

A Target with registered deployments or owned runtime resources cannot be deleted implicitly.

Mutation and destructive operations identify the full effective deployment:

```text
target + application + environment
```

## Listing deployments

`baha app list` reads the deployment registry for the effective Target rather than repository-local state.

`baha app list --all-targets` presents the installation-wide view.

CLI, JSON and MCP expose the same Target/deployment identity.

This v0.4.15 foundation is tracked in issue #408 and is intentionally designed so later Runtime Provider work in #396 and Kubernetes/OpenShift implementations can consume it without changing portable application intent.
