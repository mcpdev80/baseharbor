# Fahrplan

Die detaillierte Planung steht in [baseharbor-plan/ROADMAP.md](https://github.com/mcpdev80/baseharbor-plan/blob/main/ROADMAP.md). Diese Seite zeigt die Produktrichtung; historische Issue-Links erhalten Diskussionen und Kontext.


## Übergang der Planung

Die Planung liegt in [baseharbor-plan](https://github.com/mcpdev80/baseharbor-plan), aktiviert mit `47474109ea69475e60be4f065ad17476c71665bb`. Für Umfang, Abhängigkeiten, Zuständigkeit und Übergaben gelten die dortige `AGENTS.md` und die zugewiesene Task-Notiz. Technische Entwicklungsregeln, PR-Reviews und authentifizierte Release-Nachweise bleiben in ihren bisherigen Repositories. Wegen Migration geschlossene Issues sind historische Verweise und kein Umsetzungsnachweis.


## Aktuell — Entwicklung von v0.4.26

[v0.4.25 ist veröffentlicht](releases/v0.4.25.md). Im Fokus stehen jetzt der bedarfsorientierte App-Wizard, die Bearbeitung von Capabilities und begrenzte Verbesserungen für Entwicklung und CI (#873/#888). Entwicklungsfortschritt bedeutet keine Pre-Release-Bereitschaft.

## Vor dem v0.5-Freeze

Die verbindliche Aufteilung steht in [#427](https://github.com/mcpdev80/baseharbor/issues/427):

- v0.4.26: TUI und Entwicklerabläufe (#873).
- v0.4.27: Onboarding, Profile und Node-Authority (#874).
- v0.4.28: Console-API und UX (#875).
- v0.4.29: Host-Plattformen einschließlich WSL2/macOS (#876).
- v0.4.30: natives TLS, begründete Proxys und Grenzen des Angriffsschutzes (#904).
- v0.4.31: finale semantische Abnahme (#174).

## v0.5

Gefriert, versioniert und verhärtet die öffentlichen Aufträge.

Keine neue Plattform Primitive gehört in die v0.5 Linie.

## Später

### v0.6

Übertragbarkeit nach dem Einfrieren/Garantieverhärtung und Vorbereitungsarbeiten, die nicht
erfordern die Änderung der gefrorenen tragbaren Anwendung Semantik.

Die ursprüngliche portable-Verfügbarkeit Vertrag Arbeit wurde vorgezogen in v0.4.21
V0.6 darf also kein zweites Verfügbarkeitsmodell einführen.

### v0.7

Kubernetes Runtime-Implementierung hinter der gefrorenen Runtime Provider-Grenze.

Namespace-only-Operation bleibt das primäre Autorisierungsprofil.

### v0.8

Kubernetes Complete: voller Lebenszyklus, Fähigkeit/Anbieter, Erholung,
Beobachtungsfähigkeit, Update/Migration, Agent/DX und produktionshärtende Parität.

OpenShift, Enterprise und Cloud/Runtime-Erweiterung bauen auf dem eingefrorenen portablen
die Verträge zu ändern, anstatt sie zu ändern.

