---
hide:
  - toc
---

<img class="bh-home-mark" src="../brand/baseharbor-master-lockup.png" alt="BaseHarbor — Build · Run · Control · Everywhere">

# Deine Anwendungen. Deine Infrastruktur.

BaseHarbor verbindet Anwendungen und ihre Backends auf verifizierten Linux-Targets. Mit einer CLI betreibst du native Workloads, verbindest sichere Provider-Bindings und prüfst das Ergebnis.

[Jetzt starten](tutorials/getting-started.md){ .md-button .md-button--primary }
[GitHub](https://github.com/mcpdev80/baseharbor){ .md-button }

Dokumentation für **BaseHarbor v0.4.26**. Veröffentlichte Versionen und Installationsdateien findest du in den [GitHub-Releases](https://github.com/mcpdev80/baseharbor/releases).

## Vom Repository zur laufenden Anwendung

Installiere die [CLI und eine Rootless-Docker- oder Podman-Runtime](tutorials/getting-started.md) und öffne das Repository deiner Anwendung:

```bash
baha init
baha up
baha status
```

`baha init` übernimmt das Repository und speichert die systemseitig vergebene Application-Identität. `baha up` stellt angeforderte Provider und ihre echten Abhängigkeiten bereit oder verwendet vorhandene. Eine unterstützte lokale Anwendung ohne angeforderte verwaltete Dienste benötigt keine unbeteiligten Service-Container und keinen Pflicht-Login. Ein ausdrücklich aktivierter Management-Core oder eine Console hat eigene Sicherheitsabhängigkeiten. Status unterscheidet verifizierte Readiness von einem lediglich laufenden Workload.

## Mit deinen gewohnten Werkzeugen entwickeln

Anwendungscode und Bibliotheken bleiben erhalten. BaseHarbor erkennt unterstützte Compose-, Quadlet- und Kubernetes-Workload-Quellen, löst providerneutrale Anforderungen auf und liefert geschützte Bindings. Mehrdeutige oder [nicht unterstützte Quellen](explanation/workload-sources.md) erfordern vor dem Deployment eine konkrete nächste Aktion.

## Mit gemeinsamen oder isolierten Providern betreiben

Wähle eine unterstützte Provider-Platzierung und ein verwaltetes lokales oder registriertes Remote-Target. Ownership, TLS, Berechtigungen und erhaltene Daten bleiben im gesamten Lifecycle ausdrücklich geprüft. Die [Core-Ressourcenbeobachtungen](explanation/core-resources.md) unterscheiden gemessene Referenz-Topologien von Planungsbudgets und dem zusätzlichen Verbrauch durch Isolation.

## Kontrollieren und verifizieren

Prüfe vor Änderungen den Plan, diagnostiziere Fehler mit Doctor und kontrolliere Backups vor einer Recovery. CLI, JSON und MCP verwenden dieselben Domänenoperationen. [EN und DE](https://mcpdev80.github.io/baseharbor/) haben gleichwertige Seiten- und Befehlsabdeckung; ausgelieferte technische Bezeichner und verifizierte CLI-Ausgaben bleiben identisch.

## Wähle deine nächste Aufgabe

| Aufgabe | Einstieg |
| --- | --- |
| Eine Anwendung erstellen, übernehmen oder betreiben | [Applications](cli/applications.md) |
| Ein lokales oder entferntes Deployment-Ziel wählen | [Targets](cli/targets.md) |
| Verwaltete oder externe Backends anbinden | [Provider](explanation/providers.md) |
| Identity, TLS und Trust verwalten | [Sicherheit](explanation/security.md) |
| Über JSON und MCP automatisieren | [Automation und Agents](cli/automation-agents.md) |
| Ausgelieferte Befehle und Verträge nachschlagen | [Befehlsindex](cli/command-index.md) · [Referenz](reference/cli.md) |

Ausgelieferte Änderungen und Support-Grenzen stehen in den [Release Notes](releases/index.md).
