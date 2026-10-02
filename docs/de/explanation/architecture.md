# Architektur

BaseHarbor übersetzt portable Anforderungen einer Anwendung in verifizierte Infrastruktur.

```text
Anwendung
   ↓
Portable Anforderungen
   ↓
Umgebung + Richtlinie
   ↓
BaseHarbor Core
   ↓
Runtime- + Fähigkeits- + Auslieferungs-Provider
   ↓
Verifiziertes Ergebnis
```

## Portable Anforderungen

Die Anwendung beschreibt, was sie braucht. Sie schreibt nicht vor, welches Infrastrukturprodukt das umsetzen muss.

## Drei Provider-Achsen

```text
runtime != capability != delivery
```

- Runtime-Provider: wo Workloads laufen.
- Fähigkeits-Provider: wie eine fachliche Infrastruktur-Anforderung umgesetzt wird.
- Auslieferungs-Provider: wie der gewünschte Runtime-Zustand ausgerollt und abgeglichen wird.

Aktuell verwendet Docker Docker Compose. Podman übersetzt dieselben Workload-/Runtime-Anforderungen in native Quadlets und verwaltet sie ohne Root-Rechte über `systemd --user`. `podman-compose` ist dafür nicht erforderlich. Kubernetes und OpenShift folgen später.

## Besitz und Platzierung

Wo anwendbar:

```text
application
shared
external
```

BaseHarbor verändert nur Ressourcen, die es besitzt.

## Lebenszyklus

```text
planen -> vorprüfen -> anwenden -> verifizieren
```

CLI, JSON und MCP benutzen dieselbe Semantik.

Normative Details stehen ausschließlich in den englischen [Spezifikationen](https://mcpdev80.github.io/baseharbor/spec/).
