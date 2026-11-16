# Podman Compose network alias scope

## Symptome

Ein Dienst arbeitet unter Docker Compose, kann aber eine Abhängigkeit durch seinen erwarteten Alias unter Podman Quadlet nicht beheben.

## Ursache

Compose Aliases werden auf ein einzelnes Netzwerk scoped. Ein globales Quadlet `NetworkAlias=` Der Eintrag bewahrt dieses Modell nicht, wenn ein Container mehrere Netzwerke verbindet.

## Behebung

Aliases als Optionen auf dem einzelnen Netzwerkanhang wiederherstellen:

```text
Network=<network>.network:alias=<service>:alias=<compose-alias>
```

Externe Netzwerke verwenden ihren eigentlichen Netzwerknamen anstelle eines generierten `.network` Einheit.

Relevante Zusagen:

- `11c4b065`
- `3c17c856`

Regressionstests leben in `internal/providers/runtime/podman/quadlet_project_test.go`.
