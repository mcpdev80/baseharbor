# BaseHarbor-Dokumentation

BaseHarbor ist eine sichere, modulare und selbst gehostete Backend-Infrastruktur für unabhängige Anwendungen. `baha` übernimmt Bereitstellung, Isolation, Secrets, Lebenszyklus und Wiederherstellung; die Anwendung selbst verwendet weiterhin Standardprotokolle, Umgebungsvariablen und Dateien.

> BaseHarbor soll operative Komplexität verbergen, aber keine Standard-Schnittstellen verstecken.

## Einstieg

- [Repository-Workflow](repository-application-workflow.md)
- [Anwendungsvertrag](application-contract.md)
- [`baha` CLI](cli.md)
- [Control Plane](runtime-compose.md)
- [Secrets und OpenBao](secrets-and-openbao.md)
- [Backup und Restore](backup-and-restore.md)
- [Releases](releases.md)
- [Architektur](architecture.md)
- [Capability- und Provider-Modell](capability-provider-model.md)
- [ADR: Application Contracts beschreiben Capabilities statt Produkte](../decisions/0005-capabilities-not-products.md)
- [Roadmap](roadmap.md)

Die englische Dokumentation ist die kanonische Quelle für den öffentlichen Vertrag. Diese deutsche Fassung wird zusammen mit ihr gepflegt. Bei Abweichungen gelten Code, Acceptance-Tests und die englische Release-Dokumentation als unmittelbare Referenz.
