# PostgreSQL und Migrationen

BaseHarbor verwendet PostgreSQL als primären dauerhaften Datenspeicher der Control Plane.

## Treiber

Die Datenbankschicht verwendet direkt `pgx/v5` und `pgxpool`. In dieser Phase wird kein ORM eingeführt.

Die Verbindungsschicht:

- verarbeitet PostgreSQL-DSNs;
- unterstützt explizite minimale und maximale Poolgrößen;
- begrenzt die Wartezeit beim initialen Verbindungsaufbau;
- meldet erst nach einem erfolgreichen `Ping` Bereitschaft;
- schließt den Pool, wenn die Startverifikation fehlschlägt.

## Migrationen

SQL-Migrationen aus `internal/database/migrations` sind in das BaseHarbor-Binary eingebettet.

Vorwärtsmigrationen behalten ihre stabilen historischen Dateinamen, beispielsweise:

```text
0001_core_identity.sql
0002_tenant_rls.sql
```

Optionale Rollbacks verwenden denselben Versionsstamm mit `.down.sql`:

```text
0001_core_identity.down.sql
0002_tenant_rls.down.sql
```

Der Vorwärts-Runner:

1. legt bei Bedarf `baseharbor_schema_migrations` an;
2. lädt die eingebetteten Vorwärtsdateien `.sql`, jedoch keine `.down.sql`;
3. führt sie in lexikografischer Reihenfolge aus;
4. überspringt bereits registrierte Versionen;
5. führt jede Migration innerhalb einer Transaktion aus;
6. protokolliert die Migration erst in derselben erfolgreich abgeschlossenen Transaktion.

Fehlgeschlagene Migrationen werden daher nicht als angewendet markiert.

`RollbackLast` macht ausschließlich die zuletzt angewandte Migration rückgängig und verlangt dafür eine explizit passende Rollback-Datei. Die Rücknahme des Schemas wird niemals geraten. Rollback-SQL und Entfernen des Migrationseintrags erfolgen in einer gemeinsamen Transaktion.

Eine Rollback-Migration darf nur Objekte oder Einstellungen entfernen, die ihre korrespondierende Vorwärtsmigration angelegt hat. Eine spätere Rücknahme darf das Schema früherer Migrationen nicht löschen.

## Core-Schema

Das Core-Schema enthält derzeit nur grundlegende Identitäts- und Mandantenobjekte:

- `tenants`
- `external_identities`
- `memberships`

Anwendungsspezifische Tabellen gehören nicht in das BaseHarbor-Core-Schema.

## Datenbankseitige Mandantentrennung

Mandantengebundene Datenbankzugriffe verwenden einen transaktionslokalen PostgreSQL-Kontext.

`WithTenantTx` validiert die Mandanten-UUID, beginnt eine Transaktion und setzt:

```sql
SELECT set_config('baseharbor.tenant_id', '<tenant-uuid>', true);
```

Durch das abschließende `true` gilt die Einstellung ausschließlich für die Transaktion. Nach Commit oder Rollback wird sie automatisch verworfen und kann deshalb nicht als Mandantenkontext in einer gepoolten Verbindung verbleiben.

Für `memberships` ist Row-Level Security (RLS) aktiviert und erzwungen. Die Policy vergleicht `tenant_id` mit `current_setting('baseharbor.tenant_id', true)::uuid` bei Lese- und Schreibzugriffen.

Daraus folgt:

- Ohne Mandantenkontext sind keine Membership-Zeilen sichtbar.
- Mandant A kann keine Memberships von Mandant B lesen.
- Mandant A kann keine Zeilen für Mandant B einfügen oder aktualisieren.
- Der Anwendungscode muss nicht bei jeder Abfrage manuell ein Mandantenprädikat ergänzen.
- `FORCE ROW LEVEL SECURITY` verhindert auch bei gewöhnlichen Tabelleneigentümern eine unbeabsichtigte Umgehung der Policies.

Globale Tabellen wie `tenants` und `external_identities` sind aktuell nicht mandantengebunden. Neue mandantengebundene Tabellen im Core oder in Modulen benötigen vergleichbare RLS-Policies in der Migration, die sie einführt.

## Datenbankrollen

`deploy/postgres/roles.sql` definiert zwei Capability-Rollen ohne Login:

- `baseharbor_runtime`
- `baseharbor_migrator`

Für beide gelten ausdrücklich `NOSUPERUSER` und `NOBYPASSRLS`.

Runtime-Dienstidentitäten sollen ausschließlich die Capability `baseharbor_runtime` erhalten. Administrative und Bootstrap-Zugangsdaten dürfen nicht für die gewöhnliche Verarbeitung von BaseHarbor-Anfragen verwendet werden.

Der Rollenabgleich ist von Schema-Migrationen getrennt, da PostgreSQL zur Rollenerstellung erhöhte Clusterrechte verlangt. Der vorgesehene `baha`-Provisionierungsablauf erstellt beziehungsweise aktualisiert Rollen über eine administrative Verbindung, führt Migrationen mit einer eigenen Migrationsidentität aus und gleicht anschließend die Runtime-Berechtigungen ab.

## Verifikation

Die CI verwendet einen echten PostgreSQL-Dienst. Integrationstests belegen:

- Ohne Mandantenkontext sind keine Membership-Zeilen sichtbar.
- Mandant A sieht ausschließlich seine eigenen Zeilen.
- Direkte Lesezugriffe auf Daten von Mandant B aus Kontext A bleiben verborgen.
- PostgreSQL weist mandantenübergreifende Schreibzugriffe zurück.
- Fehlerhafte Mandantenkennungen werden vor Einrichtung des Mandantenkontexts abgewiesen.
- Der Rollback der RLS-Migration lässt das zuvor angelegte Core-Identitätsschema bestehen.
- Der Rollback der Core-Identitätsmigration entfernt ausschließlich ihr eigenes Schema.

Mandantentrennung und die Eigentümerschaft von Rollback-Operationen werden somit an echten PostgreSQL-Grenzen getestet, nicht nur durch Anwendungskonventionen.
