# Podman Quadlet rootless Speicherkontext

## Unterschrift

```text
Quadlet network resource <name> is missing after restarting <unit>-network.service
```

Das gleiche Muster kann Volumen beeinflussen.

## Symptome

Eine generierte Quadlet-Ressourceeinheit startet oder startet neu, aber der Follow-up Podman CLI-Ressource-Check sieht das Netzwerk oder die Lautstärke nicht.

Docker ist nicht betroffen, weil der Docker Compose-Pfad die rootless Podman CLI/systemd-Grenze nicht überquert.

## Ursache

Der Rootless Podman Zustand ist sensibel für die Prozessumgebung und Speicherkonfiguration. Der interaktive BaseHarbor Prozess und der systemd User Manager, der generierte Quadlet Einheiten ausführt, müssen dieselbe relevante XDG und Container-Speicherumgebung verwenden.

## bereits angewandte Fixierungen

- `5568542f` propagiert Podman-Speicherumgebung in generierte Quadlet-Einheiten.
- `81d3627d` zusätzliche Regressionsabdeckung für diese Umweltausbreitung.

## Gegenwärtiger Stand

Untersuchen. Wenn die Signatur immer noch mit beiden vorhandenen Fixes angezeigt wird, überprüfen Sie, ob die Ressourceneinheit die Ressource tatsächlich erstellt, bevor Sie Timeouts ändern.

## Überprüfung

```text
go test ./internal/providers/runtime/podman
targeted Podman guided gate
targeted Podman identity gate
```
