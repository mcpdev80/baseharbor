# Sicherheit

BaseHarbor behandelt Sicherheit als Verhalten, nicht als Etikett.

Die wichtigsten Regeln:

- bei Unsicherheit sicher abbrechen;
- minimale notwendige Rechte;
- Besitz explizit prüfen;
- keine Geheimnisse in normalen Ausgaben;
- generierte Zugangsdaten bleiben geschützter Zustand;
- keine Mutation ohne Plan und Vorprüfung;
- Erfolg erst nach Verifikation.

Die Entwicklungsumgebung darf bequem sein, aber weder Isolation noch Besitzprüfung oder Geheimnissicherheit abschalten.

Normative Sicherheitsregeln stehen in den englischen [Sicherheitsinvarianten](https://mcpdev80.github.io/baseharbor/spec/security-invariants/).

## Administrationsgrenze bei gemeinsam genutzten Datenbanken

Gemeinsam genutzte Infrastruktur bedeutet niemals gemeinsam genutzte Zugangsdaten.

Beim gemeinsam genutzten PostgreSQL-Provider ist `baseharbor_admin` ausschließlich eine interne Zugangsdaten-Identität der Steuerungsebene. Workloads erhalten nur ihre eigene App-Rolle, ihr eigenes Passwort und ihre eigene Datenbankbindung. Provider-Administrationszugangsdaten erscheinen weder in Anwendungsbindungen noch in Umgebungsverträgen, Status, Doctor, Nachweisen oder normalen Diagnosen.

Destruktive Operationen werden aus der Registrierung und dem geschützten Provider-Zustand autorisiert und schlagen bei unklarem Besitz fehl. Vor dem Löschen muss BaseHarbor nachweisen, dass Datenbank und Rolle exakt zur registrierten Kombination aus Anwendung, Umgebung und SQL-Instanz gehören.

## Geschützte HTTP- und Terminal-Sitzungen

Browser und Console verwenden einen fest gebundenen HTTPS-Endpunkt und den
verifizierten Operator. Credentials gehören weder in URLs noch in Redirects.
Ein fremder Browser-Origin wird vor der Operation abgewiesen. Auch in `dev`
bleibt ein angemeldeter Operator als eigener Actor erhalten; lokales CLI-Arbeiten
benötigt weiterhin keinen Pflicht-Login.

Der interaktive Container-Terminalpfad prüft Ressourcenbesitz und Umgebung,
bevor er den ausgewählten Runtime-Transport öffnet. Input und Resize verwenden
eine fortlaufende Sequenz; nach einem unklaren Schreibfehler darf Input nicht
automatisch wiederholt werden. Verbindungen enden spätestens nach fünf Minuten
oder beim früheren Token-Ablauf. Ein neuer Token verlängert keine bestehende
Terminal-Sitzung. Sofortiger Gruppenwiderruf ist damit noch nicht zugesichert.

Die vollständige v0.4.23-Integration wird noch qualifiziert. Fokussierte Source-
und echte Browser-Prüfungen belegen bereits Actor-Isolation, PTY-Input/Resize/Exit,
Log-Follow und physischen Verbindungsabbruch. Diese Teilnachweise sind keine
Freigabe des vollständigen Setup-/Application-/Rotationsablaufs. Die normative
[HTTP-Spezifikation](https://mcpdev80.github.io/baseharbor/spec/machine-http-v1/)
beschreibt Protokoll, Grenzen und sicheren nächsten Schritt.
