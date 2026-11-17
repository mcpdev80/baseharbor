# Fahrplan

Die detaillierte Planung steht in [baseharbor-plan/ROADMAP.md](https://github.com/mcpdev80/baseharbor-plan/blob/main/ROADMAP.md). Diese Seite zeigt die Produktrichtung; historische Issue-Links erhalten Diskussionen und Kontext.


## Übergang der Planung

Die Planung liegt in [baseharbor-plan](https://github.com/mcpdev80/baseharbor-plan), aktiviert mit `47474109ea69475e60be4f065ad17476c71665bb`. Für Umfang, Abhängigkeiten, Zuständigkeit und Übergaben gelten die dortige `AGENTS.md` und die zugewiesene Task-Notiz. Technische Entwicklungsregeln, PR-Reviews und authentifizierte Release-Nachweise bleiben in ihren bisherigen Repositories. Wegen Migration geschlossene Issues sind historische Verweise und kein Umsetzungsnachweis.


## Release-Umfang — v0.4.27

[v0.4.27](releases/v0.4.27.md) konzentriert sich auf die Verbindung eines Entwicklerrechners mit einem Core, den Lebenszyklus von Benutzern und Workstations, App-Deployments, Versionen je Rollout-Stufe, Target-Deployment-Richtlinien und die Referenz-Demo. Veröffentlichte Versionen findest du in den [GitHub-Releases](https://github.com/mcpdev80/baseharbor/releases).

## Vor dem v0.5-Freeze

Die aktuelle Reihenfolge:

- v0.4.28: einheitliche CLI-Befehle, Optionen und Bedienung.
- v0.4.29: Unterstützung für WSL2 und macOS nach der CLI-Überarbeitung.
- v0.4.30: natives TLS, begründete Proxys und Grenzen des Angriffsschutzes.
- v0.4.31: finale semantische Abnahme.

Console und der eigenständige Node Connector sind pausiert und nicht Teil des öffentlichen Release-Umfangs.

## v0.5

Friert die öffentlichen Schnittstellen ein, versioniert und härtet sie.

Neue Plattform-Grundlagen gehören nicht in die v0.5-Reihe.

## Später

### v0.6

Übertragbarkeit und Garantiehärtung nach dem Freeze sowie Vorbereitungsarbeiten,
für die keine Änderung der eingefrorenen portablen Anwendungssemantik nötig ist.

Die ursprüngliche Arbeit am Vertrag zur portablen Verfügbarkeit wurde auf v0.4.21
vorgezogen. v0.6 führt daher kein zweites Verfügbarkeitsmodell ein.

### v0.7

Kubernetes-Runtime-Implementierung hinter der eingefrorenen Runtime-Provider-Grenze.

Namespace-only-Operation bleibt das primäre Autorisierungsprofil.

### v0.8

Kubernetes Complete: vollständiger Lebenszyklus, Capabilities und Provider,
Wiederherstellung, Observability, Updates und Migration sowie Parität bei Agenten,
Entwicklererfahrung und Produktionshärtung.

OpenShift, Enterprise und weitere Cloud- und Runtime-Erweiterungen bauen auf den
eingefrorenen portablen Verträgen auf.

