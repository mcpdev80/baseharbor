# Plattformunterstützung

Unterstützung ist immer an ein bestimmtes Betriebssystem, eine Architektur und eine Runtime gebunden. Ein erfolgreicher Cross-Build oder ein veröffentlichtes Binary **beweist keinen erfolgreichen Runtime-Betrieb**. Die folgende Tabelle dokumentiert den veröffentlichten Ausgangsstand v0.4.22; eine spätere Qualifizierung muss an den exakten Release-Kandidaten gebunden sein.

| Host / Ausführungsumgebung | Runtime | amd64 | arm64 | Nachweis / Einschränkung |
| --- | --- | --- | --- | --- |
| Natives Linux | Docker / Compose | Release-qualifiziert | Build und Artefakt veröffentlicht, Runtime unbewiesen | [v0.4.22 Pre-Release](https://github.com/mcpdev80/baseharbor/actions/runs/37385682137): Atomic, HA, vollständige Journey |
| Natives Linux mit User-systemd | Rootless Podman / Quadlet | Release-qualifiziert | Build und Artefakt veröffentlicht, Runtime unbewiesen | Getrennte Podman-Nachweise im selben Pre-Release; User-systemd erforderlich |
| WSL2-Linux-Gast | Docker / Podman | Host-Kombination nicht nachgewiesen | Nicht nachgewiesen | Ein Linux-Binary belegt weder WSL-Dienstbetrieb noch Netzwerk- oder Terminalverhalten |
| Natives Windows | Beliebig | Kein unterstütztes Release-Binary und keine qualifizierte Runtime | Nicht unterstützt | Kein natives Windows-Core-Artefakt oder nachgewiesener Lifecycle |
| Natives macOS | Beliebig | Kein unterstütztes Release-Binary und keine qualifizierte Runtime | Nicht unterstützt | Linux-VM und Remote-Target sind ein anderer Host-Kontext |
| Linux | Kubernetes / K3s | Architektur-/Adoption-Nachweis | Nicht nachgewiesen | Quellcode-/Adoption-Tests stellen keinen unterstützten Runtime-Provider bereit |
| Linux | OpenShift / OKD | Geplant | Nicht nachgewiesen | Namespace-Design allein begründet keine Runtime-Unterstützung |
| Remote-Linux-Node | [Node-Connector](https://github.com/mcpdev80/baseharbor-node-connector) → Docker / Podman | Integrationsstand ist separat nachzuweisen | Nicht nachgewiesen | Connector-Tests ersetzen keine an exakte SHAs gebundenen Core→Connector→Runtime-Nachweise |
| Browser | [Console](https://github.com/mcpdev80/baseharbor-console) → HTTPS-Core | Teilbereiche live nachgewiesen | UI hostunabhängig, Browsernachweise separat | Authentifizierung, Inventar, Terminal und Logs sind Teilbereiche; finale Integrations-Pins müssen separat geprüft werden |

Die Linux-amd64/-arm64-Artefakte sind in `.goreleaser.yaml` definiert. Die [Veröffentlichung für v0.4.22](https://github.com/mcpdev80/baseharbor/actions/runs/37394723884) beweist die Bereitstellung, nicht die Runtime-Abnahme auf jeder Plattform.

Für eine neue Plattform müssen Lifecycle, Security, Ownership, Recovery und Cleanup auf der konkreten Host-/Runner-Kombination nachgewiesen werden. Browser-Versionen benötigen eigene UI- und Authentifizierungsnachweise.

Die Companion-Repositories sind optionale Implementierungen hinter den Core-eigenen Verträgen: Die Console greift niemals direkt auf eine Runtime oder den Node-Connector zu. Der Connector führt ausschließlich begrenzte, durch den Core ausgewählte und autorisierte Operationen aus. Gewünschter Zustand, Autorisierung, Platzierung und Reconciliation verbleiben im Core. Eigene Repository-CI genügt daher nicht als Freigabe für BaseHarbor.
