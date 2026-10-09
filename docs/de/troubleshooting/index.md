# Fehlerbehebungs-Wissensbasis

BaseHarbor hält wiederkehrende Laufzeit- und Akzeptanzausfälle in einer versionierten Repository-Wissensdatenbank.

Verwenden Sie dieses Verzeichnis, bevor Sie eine neue Docker, Podman, TLS, Identität, Anbieter oder Abnahmeuntersuchung starten.

## Arbeitsablauf

1. Suchen `.github/known-failures.yaml` für eine Fehlersignatur.
2. Öffnen Sie das passende Ereignisdokument in `docs/troubleshooting/incidents/`.
3. Überprüfen Sie die aufgezeichneten Ursache, Korrekturen, damit zusammenhängende Probleme und Regressionstests.
4. Überprüfen Sie, ob der aktuelle Zweig bereits den dokumentierten Fix enthält.
5. Starten Sie erst dann eine neue CI-Untersuchung, wenn der Fehler neu ist oder der dokumentierte Fix vorhanden ist und sich der Fehler immer noch reproduziert.

## Vorfälle

- [Podman Quadlet rootless storage context](incidents/podman-quadlet-rootless-storage-context.md)
- [Podman local candidate image resolution](incidents/podman-local-candidate-image-resolution.md)
- [Podman network alias scope](incidents/podman-compose-network-alias-scope.md)
- [Developer gateway effective port](incidents/dev-gateway-effective-port.md)
