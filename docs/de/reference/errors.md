# Fehler und Ergebnisse

BaseHarbor verwendet typisierte Ergebnisse, damit Menschen und Agenten Fehlerursachen eindeutig unterscheiden können.

Das semantische Ergebnis eines Befehls bildet sich auf einen einzelnen stabilen Prozess-Exit-Code ab. Siehe [Process exit codes](cli.md#process-exit-codes) für die vollständige dokumentierte Menge (`0` Erfolg, `1` Laufzeitfehler, `2` Nutzungsfehler, `130` SIGINT, `143` SIGTERM sowie den child-status passthrough `1-255` von `baha app exec`).

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
