# ADR 0006: Tragbarer Bewerbungsvertrag vor Laufzeitanbietern

> Kompatibilitätsstatus: Voreinfrieren Legacy/Migration-Preservation Wortlaut unten wird ersetzt durch[ADR 0018](0018-public-contract-namespace-and-compatibility.md). Die historische Begründung bleibt erhalten; v0.4 trägt keine Vermächtnis- oder Migrationspflicht.

Status: angenommen

## Kontext

BaseHarbor v0.3.0 vervollständigt den Compose-Lebenszyklus für den lokalen/selbstgehosteten Betrieb. Der nächste Architekturschritt muss diesen Arbeitsweg bei der Vorbereitung der gleichen logischen Anwendungsanforderungen für Kubernetes, OpenShift und externe Leistungsanbieter beibehalten.

Das aktuelle Manifest v1 enthält sowohl portable Application Intent als auch Deployment/Provider-spezifische Details. Beispiele für portable Intent sind relationale Datenbankanforderungen, benannte Key-Value/Cache-Anforderungen und erforderliche Geheimnamen. Beispiele für nicht portable Details sind `app.environment` als Bereitstellungskontext und das aktuelle Repository Auswahl der Workloads komponieren.

Wenn Runtime/Provider-Details zur Applikations-API werden dürfen, würde Kubernetes/OpenShift-Unterstützung erfordern, dass Anwendungen reoperationalisiert werden, anstatt einfach von einem anderen Anbieter realisiert zu werden.

## Entscheidung

BaseHarbor stellt einen expliziten Provider-neutral vor `PortableContract` Domain-Ansicht zwischen manifester Kompatibilität und Runtime/Capability Realisation.

Der Vertrag enthält anwendungseigene logische Identität und tragbare Fähigkeit Absicht nur. Die anfängliche v0.4 Naht stellt:

- relationale SQL-Ressourcen als `database.sql` Fähigkeiten;
- cache/key-value-Ressourcen als `cache.key-value` Fähigkeiten;
- Managed-secret Intention und erforderliche/generierte geheime Erklärungen ohne Auswahl eines geheimen Produkts.

Die folgenden gehören nicht in den portablen Vertrag:

- Umwelt-/Beschäftigungskontext;
- Komponieren Sie Projekt-, Service-, Netzwerk-, Volumen- oder Hostportnamen;
- Kubernetes Namespaces, Deployments, StatefulSets, Services, PVCs, Ingress/Gateway-Objekte oder geheime Objekte;
- OpenShift-Routen, SCC-Details oder Operator-spezifische Ressourcennamen;
- OpenBao/Vault-Pfade, AppRolles oder Provider-Anmeldeinformationen;
- TLS-Quellverzeichnisse oder anderer Provider-lokaler Implementation-Zustand.

Das Manifest v1 bleibt eine unterstützte Kompatibilitätsoberfläche.`PortableContractFromManifest` v0.4 muss diese Naht inkrementell entwickeln, anstatt den manifesten/runtime Pfad der Arbeit v0.3 in einem Umschreiben zu ersetzen.

Laufzeitanbieter und Leistungsanbieter sind separate Achsen:

```text
Application source / manifest compatibility
                 |
                 v
        PortableContract
                 |
        +--------+---------+
        |                  |
        v                  v
 Capability providers   Runtime provider
 SQL/cache/secrets      Compose / Kubernetes / OpenShift
```

Ein Runtime-Anbieter besitzt Workload-Realisierung und provider-native Objekte. Ein Capability-Provider besitzt die Implementierung einer angeforderten Service-Fähigkeit. Beispielsweise kann ein OpenShift-Workload weiterhin einen externen PostgreSQL-Provider, Vault und Ceph RGW verwenden.

Die Auswahl des Anbieters muss die geforderten Garantien beibehalten oder eindeutig scheitern. Sie darf niemals die Sicherheit, Haltbarkeit oder Verfügbarkeit stillschweigend reduzieren.

## Folgen

- Bestehende v0.3 Compose Manifeste und CLI-Verhalten bleiben kompatibel.
- Compose wird zur ersten Anbieterimplementierung und nicht zum permanenten konzeptuellen Anwendungsmodell.
- Kubernetes/OpenShift-Unterstützung kann die gleiche portable Absicht zu nativen Primitiven abbilden, ohne Kubernetes/OpenShift-Felder zum gemeinsamen Anwendungsvertrag hinzuzufügen.
- Künftige Vertragsschemaarbeit kann sich unabhängig von Provider-Objektschemas entwickeln.
- Provider-spezifische Workload-Konfiguration bleibt draußen `PortableContract` bis ein wirklich tragbares Workload/Expositionsmodell bewusst konzipiert ist.
- Neue Fähigkeiten müssen zunächst ihre tragbaren Semantik definieren, bevor ein Standardprodukt in den Anwendungsvertrag verdrahtet wird.

## Ursprüngliche Umsetzungsgrenze

Die erste v0.4 Implementierung führt absichtlich kein generisches Plugin-Framework, vollständige Provider-Schnittstellenhierarchie oder Kubernetes/OpenShift-Implementierung ein. Es stellt und übt die kleinste Domänennaht, die benötigt wird, um weitere Provider-Leakage zu verhindern.

`BuildPlan ` verbraucht logische SQL, Schlüssel-Wert und geheime Absicht durch`PortableContract`. Repository Compose bleibt ein Pfad zur Kompatibilität zwischen Workload und Source; die Laufzeit-Provider-Grenze wird durch ADR 0007 definiert und darf Compose nicht als Provider-Identität behandeln.

## Folgemaßnahmen

Folgende Arbeiten v0.4 sollten durchgeführt werden:

1. verbleibende Manifest-v1-Felder als portable Absichtserklärung, Bereitstellungskontext oder anbieterspezifische Kompatibilitätsdaten einzustufen;
2. Festlegung der Laufzeit-Provider-Grenze um die Realisierung des Arbeitsaufwands und den Lebenszyklus;
3. Einführung von Fähigkeitenverhandlungen mit expliziten nicht unterstützten Fähigkeitenausfällen;
4. Aufbau des generischen Input-Resolvers über die Grenzen zwischen Anwendung und Bereitstellung;
5. Hinzufügen von Provider-konformen Tests, die von Docker, Podman und später Kubernetes/OpenShift Runtime Provider-Implementierungen wiederverwendbar sind.
