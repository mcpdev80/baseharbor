# ADR 0008: Entwicklung des Anwendungsvertrags und Kompatibilität

> Kompatibilitätsstatus: Voreinfrieren Legacy/Migration-Preservation Wortlaut unten wird ersetzt durch[ADR 0018](0018-public-contract-namespace-and-compatibility.md). Die historische Begründung bleibt erhalten; v0.4 trägt keine Vermächtnis- oder Migrationspflicht.

## Status

> Roadmap Sequenzierung Anmerkung (2026-09-21): die architektonische Entscheidung bleibt unverändert, aber Release Sequenzierung hat sich entwickelt. v0.7 implementiert die Kubernetes Runtime, v0.8 vervollständigt Kubernetes Feature Parität und Produktionsakzeptanz, und OpenShift/Enterprise Spezialisierung folgt danach. Versionsetiketten in diesem ADR sind Sequenzierung Kontext, nicht Teil der Vertragsentscheidung.


Akzeptiert für v0.4.0.

## Kontext

BaseHarbor muss den Weg von der lokalen Entwicklung und Single-Host-Komposition zu zukünftigen Kubernetes und OpenShift-Bereitstellungen überstehen lassen.`baseharbor.yaml` Schema ist Manifest v1 und enthält immer noch Felder, deren derzeitige Realisierung Compose-orientiert ist. Das Schema nur zu brechen, um die Implementierung Provider-neutral aussehen würde das Pre-v1-Kompatibilitätsversprechen verletzen und würde bestehende Anwendungen wieder zu operationalisieren zwingen.

ADR 0005 erfordert Aufträge zur Beschreibung von Fähigkeiten und nicht von Infrastrukturprodukten. ADR 0006 führt den Anbieter-neutral ein `PortableContract` Domain-Ansicht. ADR 0007 hält die Auswahl der Laufzeitanbieter im geschützten Bereitstellungszustand und nicht im Anwendungsvertrag.

## Entscheidung

### 1. Manifest v1 bleibt eine Kompatibilitätsfläche

v0.4 ersetzt nicht vorhandene Manifest v1 Dateien. Ein gültiges Manifest v0.2/v0.3 bleibt gültig, es sei denn, es stützte sich auf bereits ungültiges oder unsicheres Verhalten.

Die Kompatibilitätsgrenze ist ein Weg:

```text
repository baseharbor.yaml (Manifest v1 compatibility)
                    |
                    v
          PortableContract
                    |
          +---------+---------+
          |                   |
          v                   v
 capability providers    runtime provider
```

Provider-Implementierungen verbrauchen normalisierte Anwendungsabsichten. Sie dürfen keine Anwendungen erfordern, um providerspezifische Infrastrukturobjekte in den gemeinsamen Vertrag umzuschreiben.

### 2. Feldklassifizierung

Die angezeigten v1-Felder werden wie folgt klassifiziert.

Beliebiges Konzept Klassifizierung .v0.4 Behandlung .v0.4 Behandlung .v0.
| --- | --- | --- |
| `app.name` Portable Applikations-Identität als stabile logische Identität beibehalten
| `app.environment` Bereitstellungskontext zur Kompatibilität akzeptiert; keine intrinsische Anwendungsidentität und kein Laufzeit-Anbieter-Selektor
, genannt PostgreSQL Instanzen , portable SQL-Fähigkeit intent , übersetzt in `database.sql` Anforderungen
Redis/Valkey-Instanzen genannt, portable Key-Value-Fähigkeiten intent-übersetzt in `cache.key-value` Anforderungen
| `secrets.required` Namen/Generationen-Intentionen, tragbare geheime Anforderung, übersetzt ohne geheime Werte,
| `services.secrets.enabled` Portable Managed-Secret-Intent-Übersetzung ohne OpenBao/Vault-Auswahl
Workload Komponieren Sie Pfad/Service-Auswahl.Providerspezifische Kompatibilitätseingabe. . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . .
Generiert Compose networks/project names/ports/volumes.exemplar-Implementierungsdetails, nie Teil des portablen Vertrags.
FQDN, TLS-Quellverzeichnis, ausgewählter TLS-Modus, Deployment/Operator-Status, gespeichert außerhalb `baseharbor.yaml` |
Ausserhalb gespeicherter Laufzeit-Anbieter/Profil-Deployment/Plattform-Zustand `baseharbor.yaml` |
Bereitstellungsanbieter/Mode- und Abgleichseigentum · Bereitstellungs-/Operatorzustand · außerhalb gespeichert `baseharbor.yaml`; nie ein Argo/Flux-Anwendungsfeld
• OpenBao Pfade/AppRollen/Politiken • Funktionsanbieter-Implementierungsdetails • nie Teil des portablen Vertrages •

Künftige Objektspeicherung, persistente Speicherung, Belichtung, TLS-Intention, Gesundheit, Backup-Relevanz, Beobachtbarkeit, generische Eingaben und Verfügbarkeits-Intention sind portable Vertragsdomänen, wenn sie Anwendungsanforderungen ausdrücken. Ihre konkreten Anbieter-Produkte, Topologie-Objekte und Sicherheitspolitik bleiben Umwelt-/Plattformbelange.

### 3. Regeln für die Versionierung

Die Top-Level-Contract-Version ist das Kompatibilitäts-Gate.

- Optionale Felder für Zusatzstoffe können innerhalb einer unterstützten Vertragsversion eingeführt werden, wenn das Fehlen eine eindeutige sichere Bedeutung hat.
- Neue erforderliche Semantik, die durch einen älteren Leser nicht sicher dargestellt werden kann, erfordern eine neue Vertragsversion.
- Unbekannte erforderliche Vertragsversionen scheitern mit einem eindeutigen nicht unterstützten Versionsfehler geschlossen; BaseHarbor ignoriert niemals stillschweigend erforderliche Semantik.
- Anbieterspezifische Fluchtluken müssen bei Bedarf optional und explizit namespaced sein. Sie dürfen die Bedeutung von portablen Feldern nicht neu definieren.
- Geheime Werte, Provider-Anmeldeinformationen und Betreibersicherheitspolitik werden nie zu engagierten Vertragsfeldern, nur um eine Providerimplementierung zu vereinfachen.
- Logische Ressourcennamen sind stabile anwendungsbezogene Identitäten. Eine Änderung des Providers oder der Topologie darf sie nicht stillschweigend umbenennen.

### 4. Kompatibilitätsadapter sind explizit und nur durch Release Policy herausnehmbar

`PortableContractFromManifest` ist der v0.4 Kompatibilitätsadapter. Provider-neutraler Code sollte von der portablen Ansicht abhängen, anstatt neue Abhängigkeiten von Manifest v1 Produkt/Compose-Feldern zu entwickeln.

Eine spätere Vertragsversion kann erstklassige Workload/Endpoints, Objektspeicher, persistente Speicherung, Belichtung/TLS-Intent, Gesundheit, Backup/Beobachtbarkeit, Eingabe- und Verfügbarkeitserklärungen erhalten. Manifest v1 translation bleibt unterstützt, bis eine separat dokumentierte Release-Richtlinie diese explizit entfernt.

### 5. Laufzeit, Leistungsfähigkeit und Lieferer sind unabhängige Achsen

Ein Einsatz kann z. B. Folgendes kombinieren:

```text
runtime: openshift
SQL capability: external PostgreSQL
object storage: Ceph RGW
secrets: Vault
```

Kubernetes oder OpenShift dürfen daher bei der Auswahl von PostgreSQL, Geheimnissen oder Objekt-Speicher-Anbietern nicht zur Abkürzung werden.

Die Bereitstellungsauswahl ist ebenfalls Bereitstellungs-/Operatorzustand. Direkte Laufzeitmutation und delegierte/GitOps-Abgleiche müssen ohne Änderung der portablen Applikationsabsicht wählbar sein. Argo CD, Flux, Git Repository Layout, Helm-Werte und abgestimmte spezifische Ressourcen bleiben Liefer-/Laufzeit-Implementierungsdetails.

Für einen verwalteten Ressourcensatz muss es genau einen aktiven Aussöhnungsbesitzer geben.

### 6. Lifecycle Stabilität ist der Akzeptanztest

Der vorgesehene Lebenszyklus ist:

```text
application source + baseharbor.yaml
            |
            +--> laptop / homelab       (Compose)
            +--> test / staging          (Compose or future Kubernetes)
            +--> production              (provider selected by deployment)
            +--> Kubernetes              (future v0.7 provider)
            +--> OpenShift / enterprise  (future post-v0.8 specialization)
```

Der Übergang zwischen diesen Phasen kann den Bereitstellungszustand, die Providerauswahl, die Topologie und die Richtlinie ändern. Es darf nicht erforderlich sein, die logischen Fähigkeiten der Anwendung neu zu schreiben oder Kubernetes/OpenShift-Objekte in den gemeinsamen Anwendungsvertrag einzuführen.

## Folgen

- Compose bleibt eine erstklassige Implementierung ohne die konzeptionelle API.
- Bestehende Repositories arbeiten weiterhin über den Adapter Manifest v1 Kompatibilität.
- Neue Anbieter-neutrale Funktionen sollten erweitert werden `PortableContract` und seine Kompatibilität Übersetzung vor dem Hinzufügen von Provider-spezifischen Realisierung.
- Kubernetes/OpenShift kann später ohne Änderung der v0.4 architektonischen Grenze umgesetzt werden.
- Der Bruch der Vertragsentwicklung ist explizit, versioniert und fail-closed und nicht aus Implementierungsdetails abgeleitet.
