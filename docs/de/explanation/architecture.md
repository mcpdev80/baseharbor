# Architektur

BaseHarbor übersetzt portable Anforderungen einer Anwendung in verifizierte Infrastruktur.

```text
Anwendung
   ↓
Portable Anforderungen
   ↓
Environment + Policy
   ↓
BaseHarbor Core
   ↓
Runtime + Capability + Delivery Provider
   ↓
Verifiziertes Ergebnis
```

## Portable Anforderungen

Die Anwendung beschreibt, was sie braucht. Sie schreibt nicht vor, welches Infrastrukturprodukt das umsetzen muss.

## Drei Provider-Achsen

```text
runtime != capability != delivery
```

- Runtime Provider: wo Workloads laufen.
- Capability Provider: wie eine fachliche Infrastruktur-Anforderung umgesetzt wird.
- Delivery Provider: wie gewünschter Runtime-State ausgerollt und reconciled wird.

Aktuell verwendet Docker Docker Compose. Podman uebersetzt dieselben Compose-basierten Workload-/Runtime-Definitionen in native Quadlets und verwaltet sie rootless ueber `systemd --user`. `podman-compose` ist dafuer nicht erforderlich. Kubernetes und OpenShift folgen spaeter.

## Ownership und Placement

Wo anwendbar:

```text
application
shared
external
```

BaseHarbor verändert nur Ressourcen, die es besitzt.

## Lifecycle

```text
plan -> preflight -> apply -> verify
```

CLI, JSON und MCP benutzen dieselbe Semantik.

Normative Details stehen ausschließlich in den englischen [Specs](https://mcpdev80.github.io/baseharbor/spec/).
