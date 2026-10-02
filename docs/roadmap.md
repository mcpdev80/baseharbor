# Roadmap

Detailed planning lives in GitHub Issues. This page shows product direction only.

## Current — v0.4.19

v0.4.19 completes the final capability/provider round before the remaining pre-freeze cleanup releases:

- stable application/deployment/provider identity;
- messaging contracts and RabbitMQ reference provider;
- durable key-value and Valkey reference provider;
- document database and MongoDB reference provider;
- External/BYO provider onboarding and trust/certificate integration;
- Organization / Platform Configuration and distribution;
- provider/runtime/delivery boundary consistency.

## Before the v0.5 freeze

### v0.4.20

Source-neutral repository adoption and complete managed workspace/Git lifecycle.

### v0.4.21

Application-to-Application consumption contract.

### v0.4.22

MCP operator authorization and extension trust.

### v0.4.23

Freeze-readiness policy, public namespace and platform contract.

### v0.4.24

Human CLI consolidation plus final documentation/GitHub Pages information architecture.

### v0.4.25

Semantic acceptance across the completed v0.4 contract.

## v0.5

Freeze, version and harden the public contracts.

No new major platform primitive belongs in the v0.5 line.

## Later

### v0.6

Portable availability and guarantee semantics.

### v0.7

Kubernetes Runtime implementation.

### v0.8

Kubernetes Complete: lifecycle, capabilities, recovery, observability, agent parity and production hardening.

OpenShift, enterprise and cloud/runtime expansion build on the frozen portable contracts.
