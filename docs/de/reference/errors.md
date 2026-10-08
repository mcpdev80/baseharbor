# Fehler und Ergebnisse

BaseHarbor verwendet typisierte Ergebnisse, damit Menschen und Agenten Fehlerursachen eindeutig unterscheiden können.

Wichtige semantische Ergebnisse:

- `unsupported`: Eine Funktion wird vom aktuellen Provider oder Target nicht unterstützt.
- `unverifiable`: Der Zustand kann nicht zuverlässig überprüft werden.
- `denied`: Die Operation ist nicht autorisiert.
- `conflict`: Gewünschter und tatsächlicher Zustand stehen im Konflikt.
- Fremde Eigentümerschaft: Die Ressource gehört nicht zur ausgewählten Application.
- `degraded`: Die Funktion ist eingeschränkt verfügbar.
- Fehlgeschlagene Verifikation: Die angestrebte Änderung wurde nicht nachweisbar erreicht.

Fehlermeldungen müssen konkrete nächste Schritte ermöglichen und dürfen keine Secrets enthalten.

Maschinenschnittstellen liefern strukturierte Fehlerobjekte. Die menschenlesbare CLI-Ausgabe darf zusätzlichen Kontext anzeigen, aber die zugrunde liegende Semantik nicht verändern.

Siehe [Authentifizierung und API-Fehler](authentication-and-api-errors.md) sowie Fehlerbehebung in der CLI.
