# Sicherheitsinvarianten

Diese Anforderungen gelten für BaseHarbor.

- Sicherheitssensible Mehrdeutigkeit MUSS nicht geschlossen werden.
- Geheimwerte MÜSSEN NICHT in normalen Protokollen, Fehlern, Prüfprotokollen, Metriken-Etiketten, Maschinenergebnissen oder übergebenen Manifesten erscheinen.
- Eigentum MUSS vor destruktiver Mutation überprüft werden.
- Eine Ressourcenkennung zu kennen MUSS NICHT ausreichend Autorisierung sein.
- Workload Identity, Application-User Identity, Human/Operator Identity und Provider-Admin-Anmeldeinformationen MUSS eindeutig bleiben.
- Managed Anmeldeinformationen MUSS der versionierten A/B/C-Eigenschafts-Taxonomie in `credential-access-v1`; Klasse C-Maschinenidentität MUSS niemals geteilte Human-/Entwickler- oder Application-Service-Anmeldeinformationen erben.
- Vertrauenswürdige lokale `dev` MÜSSEN KEINE Bedieneranmeldung verlangen;`test ` und`prod` Anwendungsoperationen MÜSSEN ohne eine gültige OIDC-Operator-Grenze geschlossen werden.
- Gemeinsame Identitätsanbieter-Infrastruktur MÜSSEN KEINE gemeinsame Anwendung/Umweltidentitäts-Scope implizieren.
- Umweltpolitik MÜSSEN die Anforderungen an die Authentifizierung von Anwendungen stärken, MÜSSEN sie jedoch NICHT stillschweigend schwächen.
- BaseHarbor MUSS keine Endbenutzerpasswörter, TOTP-Samen oder WebAuthn/Passkey-Anmeldeinformationen als Identitätsanbieter verarbeiten oder speichern.
- Langlebige uneingeschränkte Provider-Admin-Anmeldeinformationen MÜSSEN NICHT Anwendungs-Workloads ausgesetzt sein.
- Umweltfreundlichkeit MÜSSEN Isolation, Eigentum oder geheime Sicherheits-Invarianten NICHT deaktivieren.
- Erforderliche Überprüfung MÜSSEN NICHT stillschweigend auf Prozess-Gesundheit-nur Erfolg herabgestuft werden.
