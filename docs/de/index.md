# BaseHarbor-Dokumentation

Die englische Dokumentation ist die kanonische Quelle für technische Referenzen und normative Verträge.

Die deutsche Dokumentation konzentriert sich bewusst auf leicht verständliche menschliche Erklärungen.

> **KI-generiert, menschlich spezifiziert, maschinell verifiziert.**  
> Menschliche Entscheidungen definieren Ziel, Architektur, Grenzen und Akzeptanzkriterien. KI kann die Umsetzung beschleunigen; als korrekt gilt das Ergebnis erst durch deterministische maschinelle Verifikation.

## Neu bei BaseHarbor?

[Einstieg](tutorials/getting-started.md) · [Homelab: Core + Console + Remote Podman](tutorials/homelab.md)

SQL, Secrets und Identity bilden den verpflichtenden Core; die Console ist optional.
Die [Core-Ressourcen](explanation/core-resources.md) zeigen gemessene
Referenz-Topologien und erklären den zusätzlichen Verbrauch durch Provider-Isolation.

## Verstehen

- [Architektur](explanation/architecture.md)
- [Kontexte und Deployment-Ziele](explanation/targets.md)
- [Anwendungsvertrag](explanation/application-contract.md)
- [Provider](explanation/providers.md)
- [Sicherheit](explanation/security.md)

## Kommandozeile

`baha` ist die primäre menschliche Schnittstelle. Die [CLI-Übersicht](cli/index.md) führt zu den Bereichen:

- [Core-Workflow](cli/core.md)
- [Applications](cli/applications.md)
- [Targets](cli/targets.md)
- [Provider](cli/providers.md)
- [Development und Workspaces](cli/development.md)
- [Organisation und Policy](cli/organization-policy.md)
- [Security und Trust](cli/security-trust.md)
- [Automation und Agents](cli/automation-agents.md)
- [Shell und Bedienung](cli/shell-ux.md)
- [Globale Optionen](cli/global-options.md)

## Provider- und Betriebsanleitungen

- [PostgreSQL](how-to/postgres.md)
- [Cache](how-to/cache.md)
- [Dauerhafter Key-Value-Speicher](how-to/durable-key-value.md)
- [Dokumentdatenbank](how-to/document-database.md)
- [Messaging](how-to/messaging.md)
- [Objektspeicher](how-to/object-storage.md)
- [Secrets](how-to/secrets.md)
- [Observability](how-to/observability.md)
- [Externe/BYO-Provider](how-to/external-providers.md)
- [Backup und Restore](how-to/backup-restore.md)

## Plattform und Sicherheit

- [Organisationskonfiguration](explanation/organization-configuration.md)
- [Repository-Workload-Quellen](explanation/workload-sources.md)
- [Authentifizierung](explanation/authentication.md)

## Genaue technische Details

Nutze die englischen kanonischen Bereiche:

- [Technische Referenz](https://mcpdev80.github.io/baseharbor/reference/cli/)
- [Spezifikationen](https://mcpdev80.github.io/baseharbor/spec/)

Übersetzungen dürfen normative Semantik nicht neu definieren.
