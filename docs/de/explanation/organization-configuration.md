# Organisations- und Plattformkonfiguration

Plattformteams verteilen gemeinsame Defaults, ohne den portablen Vertrag jeder Application umzuschreiben. Sie können Targets, Stack Profiles, Provider-Defaults, externe/BYO-Registrierungen, Trust-/Zertifikatsmaterial und Policy-Referenzen bereitstellen.

Lokale/System-Quellen, Git und OCI sind unterstützte Verteilungswege. Git-/OCI-Quellen werden vor der Verwendung auf unveränderliche Revisionen beziehungsweise Digests aufgelöst. Die effektive Konfiguration zeigt die Herkunft der Werte.

Explizite Operator-/Application-Entscheidungen bleiben von Organisations-Defaults getrennt. Verbindliche Policy ist eine eigene Schicht; ein Default darf sie nicht überschreiben. Fehlt oder ändert sich Organisationskonfiguration, bleibt Application Intent portabel.

Weiter: [Konfiguration und Policy](../cli/organization-policy.md), [normative Auflösung (EN)](https://mcpdev80.github.io/baseharbor/spec/organization-resolution-v1/).
