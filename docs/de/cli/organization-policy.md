# Organisation und Policy

Organisationskonfiguration liefert Unternehmens-Defaults, ohne Application Intent umzuschreiben. Sie kann Targets, Stack Profiles, Provider, BYO-Dienste, Trust-Material und Policy-Referenzen bereitstellen. Git-/OCI-Quellen werden auf unveränderliche Identitäten aufgelöst.

## Konfiguration prüfen

Für ein vom Plattformteam bereitgestelltes gültiges Organisationsartefakt:

```bash
baha config organization set --source local --location "$HOME/company/baseharbor-config" --environment dev
baha config organization show --environment dev -o json
baha config organization check
```

Das Verzeichnis enthält eine Organisationskonfiguration, keinen Application-Vertrag.

Im Application-Repository die Zielumgebung prüfen:

```bash
baha policy check -e test
baha policy explain -e test
```

`check` bewertet den aufgelösten Kontext; `explain` zeigt die maßgebliche Regel. Fehlende Provider oder abgelehnte Anforderungen müssen vor Apply korrigiert werden. Überschreibbare Defaults ersetzen keine verbindliche Policy.

Weiter: [Konzept](../explanation/organization-configuration.md), [exakte Befehle (EN)](https://mcpdev80.github.io/baseharbor/cli/organization-policy/).
