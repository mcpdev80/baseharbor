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
