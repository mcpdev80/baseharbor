# Lokale, entfernte und gemeinsame Entwicklung

Der Repository-Workflow bleibt gleich, egal ob der ausgewählte BaseHarbor-Core auf der eigenen Workstation oder auf einem entfernten Host läuft. Eine Management-Verbindung bestimmt, wie die CLI einen Core erreicht. Ein Target bestimmt, wohin dieser Core eine Application ausrollt. Die Application-Umgebung wählt deren Konfiguration und Zustand.

## Lokal entwickeln

Initialisiere oder übernimm im vorhandenen Application-Repository den Vertrag einmal, prüfe den Plan und konvergiere dann die ausgewählte Umgebung:

```bash
baha init
baha plan -e dev
baha up -e dev
baha status -e dev
```

Normale vertrauenswürdige lokale Entwicklung benötigt kein `baha login`. `baha init` erfasst den Repository-eigenen Application-Vertrag; `baha up` führt den Lifecycle aus. Ein Umgebungsname ersetzt weder ein Target noch umgekehrt. Siehe [Application-Befehle](../cli/applications.md) und [Umgebung auswählen](environments.md).

## Ein entfernter Core

Bereite auf dem Core-eigenen Host die Installation vor und erstelle eine private Workstation-Einladung:

```bash
baha up
baha onboard --invite
```

Übertrage die Einladung geschützt. Löse sie auf der Entwickler-Workstation ein und wähle anschließend die erzeugte Verbindung:

```bash
baha onboard --import /path/to/invitation.json
baha use <connection>
baha connections
baha target list
baha target show
```

Arbeite danach im Application-Repository mit dem ausgewählten entfernten Target:

```bash
baha init
baha plan -e dev
baha up -e dev
baha status -e dev
```

Das Enrollment verbindet diese Workstation mit dem Core und erstellt eine Geräteidentität. Es verleiht keine Operator-Rolle. Der Verbindungsalias benennt kein Core-Target um, und bei einem Fehler des Remote-Zugriffs wechselt BaseHarbor nicht auf eine lokale Runtime. Halte den lokalen Core und die entfernte Verbindung bei der Target-Auswahl auseinander. Siehe den [Workstation-Enrollment-Vertrag](../spec/zero-friction-workstation-onboarding-v0427.md) und [Target-Befehle](../cli/targets.md).

## Gemeinsamer Teamzugriff

Erstelle die Person mit ihrer ersten Workstation auf dem zuständigen Core:

```bash
baha onboard --invite
```

Für eine weitere Workstation einer bestehenden Person erstelle mit `baha workstation new` eine eigene Einladung. Jedes Gerät besitzt eine separat widerrufbare Identität. Geräte verwaltest du mit `baha workstation list` und `baha workstation revoke NAME`; Personen mit `baha user list` und dem expliziten Ablauf `baha user offboard LOGIN`.

Nachdem sich die Person, falls die Policy es verlangt, angemeldet hat, weist du ihr über den geführten Befehl das passende Zugriffsprofil und die Targets zu:

```bash
baha access assign PERSON
baha whoami
```

Die Zuweisung gilt innerhalb der ausgewählten Installation und für die gewählten Targets. Eine geteilte Organisationskonfiguration kann versionierte Defaults liefern. Sie ist weder eine Berechtigung noch eine global synchronisierte Authority oder eine Kopie des Live-Deployment-Zustands. Siehe [Security- und Trust-Befehle](../cli/security-trust.md) und [Organisationskonfiguration](../explanation/organization-configuration.md).

## Profile, Credentials und Recovery

Die einmalige Workstation-Einladung ist ein gerätegebundenes Enrollment mit Credential-Material. Sie ist kein allgemeines Profil-Synchronisierungspaket. Für geeignete lokale Developer-Credentials einer ausgewählten Application bietet die CLI derzeit einen ausdrücklich gestarteten, verschlüsselten Export:

```bash
baha app secure-profile export --password-file /path/to/owner-only-password
```

Es gibt keinen Befehl `baha app secure-profile import`. Kopiere keine Remote-Provider-Secrets und behandle ein Verbindungsprofil nicht als Berechtigung für ein Target. Verwende bereits funktionierende native Git-, SSH-, kubeconfig- und Cloud-Credential-Chains sowie verifiziertes System- oder explizit konfiguriertes CA-Trust. Umgehe niemals einen Trust-Fehler, indem du die Zertifikatsprüfung deaktivierst. Liegt das exakte Image bereits im nativen Image-Store des Ziels, kann dieser Store ohne Registry verwendet werden. Müssen Artefakte zwischen Hosts übertragen werden, verwende eine vorhandene OCI-kompatible Registry mit ihrer konfigurierten Authentifizierung und Trust-Konfiguration. BaseHarbor installiert keine Registry stillschweigend und macht CI/GitOps für lokale Entwicklung nicht zur Voraussetzung. Siehe [Security und Trust](../cli/security-trust.md), den [Delivery-Vertrag](../spec/delivery-provider-v1.md) und [Plattformunterstützung](../reference/platform-support.md).

Application-Recovery ist ein eigener Vorgang und nicht der Transfer einer Workstation oder eines Profils:

```bash
baha app backup
baha app restore
```

Ein Backup beweist nicht, dass ein neues Application-Schema mit einem älteren Image kompatibel ist. Die Application und ihr vorhandenes Migrationstool verantworten Schemaänderungen und Kompatibilitätsnachweise; BaseHarbor führt keine allgemeine Migration Engine aus. Siehe [Backup und Restore](backup-restore.md) und den [Candidate-Vertrag](../spec/candidate-contract-v1.md).

## Exakte Versionen und Promotion-Grenze

Lesbare Release- und Component-Versionen helfen Menschen bei der Auswahl. Prüfe die erfasste Sicht mit `baha app version`; `baha app versions --all-targets` vergleicht Deployments über Targets hinweg. Gewünschte, angewendete und beobachtete Referenzen bleiben getrennte Werte. Eine fehlende Dimension wird als unklar angezeigt; weicht die Beobachtung vom zuletzt angewendeten Wert ab, ist sie veraltet. Component-Zeilen zeigen Image-Repository/-Tag und, falls verfügbar, den beobachteten Image-Digest. Mit `baha app workspace resolve -o json` kannst du die Component-zu-Repository-Provenienz prüfen.

Die angewendete unveränderliche Identität muss auf einen Git-Commit oder OCI-Digest aufgelöst werden; ein veränderlicher Git-Branch/-Tag oder Image-Tag allein genügt nicht. Eine fehlende Git-Revision bleibt unbekannt; OCI-Annotationen allein beweisen keine Git-Provenienz. Jeder Datensatz gehört zu Application, Target und Umgebung; gewünschte, angewendete und beobachtete Werte bleiben voneinander unabhängig. `stage` ist ein lesbares Label, aber keine zusätzliche Deployment-Identität oder Policy-Grenze. Die Nutzung eines exakten Candidates baut nichts und führt keine Migration aus. Die allgemeine umgebungsübergreifende Operation `baha promote` ist auf v0.4.28 verschoben und kein unterstützter v0.4.27-Befehl. `baha app update` aktualisiert unterstützte Application-Source; es ist weder Promotion noch ein Datenbank-Rollback. Siehe [Candidate-Vertrag](../spec/candidate-contract-v1.md), [Application-Befehle](../cli/applications.md) und [PostgreSQL-Migrationsgrenze](../reference/postgresql-and-migrations.md).

### Weiterlesen

- [Erste Application](../tutorials/getting-started.md)
- [Details zur Remote-Workstation](remote-workstation.md)
- [Target-Konzepte](../explanation/targets.md)
- [Maschinenlesbare Zuordnung von Befehlen und Operationen](../reference/cli-machine-coverage.md)
