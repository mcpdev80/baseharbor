# Register der öffentlichen Aufträge

Alle Einträge sind vorfrieren versionierte Entwürfe. Core-Betreuer besitzen die kanonische
Artefakte und Verbraucheränderungen koordinieren; Laufzeit/Kapazität/Lieferung/Ziel
Zugriffsimplementierungen besitzen ihre Konformität. Siehe
[compatibility policy](https://github.com/mcpdev80/baseharbor/blob/HEAD/COMPATIBILITY.md)
für Zusatzstoffe/Bruch-/Ergebnis-/Deprekationsregeln und
[ADR 0018](../decisions/0018-public-contract-namespace-and-compatibility.md)
für die namespace-Governance.

Oberfläche, kanonisches Artefakt, Version / Evolutionsgrenze,
| --- | --- | --- |
Anwendung manifest / portable Absichten[Manifest](manifest.md), [Application contract](../spec/application-contract-v1.md), `internal/application/manifest.go `, ` internal/application/contract.go`· Manifest/Anwendung v1; Anforderungen bleiben tragbar ·
Service / Fähigkeitsabsicht `contracts/service/v1/*.schema.json`, ` spec/capabilities/*/v1.md`Service/Kapazität v1; produktneutrale Intention
Verbindungsbindungen `contracts/binding/v1/service-binding.schema.json`, ` spec/bindings/service-binding-1.1.md`Verbindliche 1.1; normgebundene Verbindungsausgabe
Sichere Bindungen / Referenzen `spec/bindings/secure/v1.md`, [Credential access](../spec/credential-access-v1.md)V1; Geheimreferenzen, keine gewöhnlichen geheimen Nutzlasten
Protokoll für die Kapazität des Anbieters `spec/provider/v1/provider.proto`, [Provider contract](../spec/provider-contract-v1.md)Anbieter v1; unterscheidet sich von der Produktversion
Provider Verteilung / Deskriptor `contracts/provider/v1/provider-descriptor.schema.json` Deskriptor v1; unveränderliche Artefakte/trust-Referenzen
Laufzeitanbieter `spec/runtime-provider/v1/runtime_provider.proto`, [Runtime contract](../spec/runtime-provider-contract-v1.md)• Laufzeit v1; Realisierung getrennt von Intent
Lieferer[Delivery contract](../spec/delivery-provider-contract-v1.md)Lieferung v1; nicht Laufzeitbevollmächtigung
Arbeitslast-Quellen-Adapter `internal/repositoryinspect/workload_source.go`, [Application contract](../spec/application-contract-v1.md)Normalisiertes Quellmodell; Quelle ist nicht Laufzeit
Entwicklung Integration / StackProfile `contracts/development/v1/*.schema.json`, [Development extension](../spec/development-extension-v1.md)Entwicklung v1; ADR 0018 Profil URI-
Deskriptor der Erweiterung / Artefakt-Vertrauen `contracts/extension/v1/*.schema.json`, [Artifact trust](../spec/extension-artifact-trust-v1.md)Erweiterung/Trust v1; getrennte signierte Artefaktannahme
Organisationskonfiguration `internal/orgconfig`, [Organization configuration](../explanation/organization-configuration.md)Organisation v1;[resolution/policy](../spec/organization-resolution-v1.md)getrennt von der Verteilung
Maschinenbetrieb / JSON / MCP `internal/machine/contract.go`, [Machine interface](../spec/machine-interface-v1.md)Maschine v1; gemeinsame semantische Operationen und getippte Ausfälle
HTTP-Projektion `internal/machinehttp`, [Machine HTTP](../spec/machine-http-v1.md) | `/api/v1/machine`; gleicher Akteur/Politik/Sicherheit
Exekutionen / Ereignisse `internal/machine/execution.go` Execution/Event v1; expliziter Akteur, Zustand, Sequenz
Streams / Terminals `internal/machine/stream.go`, ` internal/machine/terminal.go `, ` contracts/machine/v1/terminal-*.schema.json`, [Machine HTTP](../spec/machine-http-v1.md)Stream v1; Implementierungsfunktionen müssen nachgewiesen werden
Laufzeit-Explorer `internal/runtimeexplorer`, [Explorer](../spec/runtime-explorer-v1.md)Explorer v1; eigentumssicherer Bestand/Betrieb
Normalisierung des Laufzeitprotokolls[Log source contract](../architecture/runtime-log-source-contract.md)Vorhandene laufzeitneutrale Protokollsemantik; kein neues Protokollmodell
Zielzugang `internal/targetaccess`, ` contracts/targetaccess/v1/wire.schema.json`, [Wire records](https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/targetaccess/README.md), [Target Access](../spec/target-access-v1.md)Zugriff v1; Deskriptor wird nicht authentifiziert Live-Verhandlungen
Managementoberflächen / Identität[Management access](../spec/management-access-v1.md), [Application identity](../spec/application-runtime-identity.md)Kontext/Vertrauensgrenzen beibehalten
Runtime Resource API.`spec/runtime-api/v1/openapi.yaml`• Laufzeit API v1; Standard-OpenAPI-Behörde
Aussöhnung / Verfügbarkeit / Verbrauch[Reconciliation](../spec/reconciliation-v1.md), [Availability](../spec/availability-v1.md), [Consumption](../spec/application-consumption-v1.md)V1; ein Kernlebenszyklus;
Prüfung / Beweis `internal/evidence`, [Audit/Evidence](../spec/audit-evidence-v1.md)V1; beibehaltener Exakt-Eingangsnachweis erforderlich
Beharrlicher Zustand / Erholung `internal/runtime/config.go`, ` internal/orgconfig/store.go`, [Backup/restore](backup-and-restore.md)Artifact-spezifische Version/Eigentum; keine Voreinfrieren-Migration versprechen

Pfade außerhalb `docs/` sind repository-relative Artefakte. Die eingebetteten
`contracts.SchemaRegistry` ist das ausführbare Schema-Inventar und löst
jedes verpackte Schema ohne Netzwerkzugriff. Menschliche Referenzen ersetzen nicht
Schemas, Go-Draht-Typen, OpenAPI- oder Protobuf-Definitionen.

Abdeckung für v0.4.23: komplette Konfigurations-Konsumenten, interaktiv
Terminallaufzeit/Browser-Qualifikation und echte Remote/Console-Integration werden in #609 verfolgt,
#806, #807 und #808. Behandeln Sie einen Eintrag in diesem Register nicht als Live-Unterstützung.
