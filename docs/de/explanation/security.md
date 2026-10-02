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

