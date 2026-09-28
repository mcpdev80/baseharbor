# Security

BaseHarbor behandelt Security als Verhalten, nicht als Label.

Die wichtigsten Regeln:

- bei Unsicherheit fail closed;
- Least Privilege;
- Ownership explizit prüfen;
- keine Secrets in normalen Ausgaben;
- generierte Credentials bleiben geschützter State;
- keine Mutation ohne Plan und Preflight;
- Erfolg erst nach Verifikation.

Dev darf bequem sein, aber nicht Isolation, Ownership oder Secret-Sicherheit abschalten.

Normative Security-Regeln stehen in den englischen [Security Invariants](https://mcpdev80.github.io/baseharbor/spec/security-invariants/).

## Administrationsgrenze bei gemeinsam genutzten Datenbanken

Gemeinsam genutzte Infrastruktur bedeutet niemals gemeinsam genutzte Credentials.

Beim Shared-PostgreSQL-Provider ist `baseharbor_admin` ausschließlich ein internes Control-Plane-Credential. Workloads erhalten nur ihre eigene App-Rolle, ihr eigenes Passwort und ihre eigene Datenbankbindung. Provider-Admin-Credentials erscheinen weder in Application Bindings noch in Environment Contracts, Status, Doctor, Evidence oder normalen Diagnosen.

Destruktive Operationen werden aus Registry/geschütztem Provider-State autorisiert und schlagen bei unklarer Ownership fehl. Vor dem Löschen muss BaseHarbor nachweisen, dass Datenbank und Rolle exakt zur registrierten Kombination aus Application, Environment und SQL-Instanz gehören.

