# Laufzeitanbieter v1 Konformität

Ein konformer Runtime Provider MUSS die gleiche tragbare Semantik unabhängig vom Anbieterprodukt nachweisen.

## Identität und Protokoll

- stabile Anbieteridentität
- Provider-Version unabhängig von Protokoll-Version
- Protokollkennung `baseharbor.runtime.provider/v1`
- explizit beworbene Laufzeit-Fähigkeiten

## Vorflug fehlgeschlagen

Nicht unterstützte Workload-Semantik MUSS vor der Mutation scheitern.

Der Anbieter MUSS Folgendes ablehnen:

- fehlende aufgelöste OCI-Bildartefakte
- Ungültiger Zielbereich, wenn ein Anwendungsbereich erforderlich ist
- Anweisungen zum Erstellen/Quellen von Repositorys an der Laufzeitgrenze
- Leistungserbringer-Felder
- Liefer-/GitOps-Eigenschaftsfelder

## Laufzeitplan

Der portable Laufzeitplan enthält nur:

- Identität der Anmeldung
- Umweltidentität
- Undurchsichtiger Zielbereich
- Identität des logischen Workload-Dienstes
- OCI Bild aufgelöst
- Args
- nicht geheime Umgebungswerte
- Workload-lokale Container-Ports
- öffentliche Bindungen oder undurchsichtige Geheimreferenzen

Der Plan MUSS KEINE provider-native Felder wie Kubernetes Namespace/Deployment, Compose project, Podman Quadlet, Docker Netzwerk oder Host Pfad enthalten.

## Beobachtung

LEADY/status/doctor/devidence semantics verbrauchen anbieterneutrale Beobachtung:

- gefunden
- läuft
- bereit
- logisches Service-Detail/Diagnostik

Provider-native Bedingung Namen sind Implementierung Details.

## Eigentum und Zerstörung

- Nur im Besitz von BaseHarbor befindliche Ressourcen dürfen vernichtet werden
- ausländische Ressourcen werden nie destruktiv eingesetzt
- Anwendung/Umwelt-Identität überlebt Anbieter-native Name Normalisierung
- wiederholt Zerstören ist sicher
- Bereinigungsnachweise dürfen keine eigenen Ressourcen nachweisen

## Trennung von Anbieter-Achsen

Runtime Provider MÜSSEN KEINE Fähigkeiten bereitstellen, nur weil die Laufzeit sie hosten kann.

Der Capability Provider bleibt für SQL, Key-Value, Dokumentendatenbank, Messaging, Objektspeicher, Geheimnisse, Identität, Metriken, Protokolle, Spuren, OTLP und Belichtung verantwortlich.

Der Lieferer bleibt für das direkte bzw. delegierte Abgleicheigentum verantwortlich.

## Nachweise

Docker und Podman sind die kompletten v0.4.x Referenzrealisierungen.

Kubernetes PR #616 ist der Crashtest vor dem Einfrieren der Architektur. Seine Erkenntnisse sind normative Belege für diesen Vertrag:

- Undurchsichtiger Zielbereich / Namespace-Separierung
- stabile Anwendungsidentität
- eigentumssichere Reinigung
- namespace-scoped operation
- Laufzeitneutrale Bindungen
- Laufzeitneutral READY/status/doctor/devidence
- keine Kubernetes-spezifische Kernsemantik

Kubernetes-Produktionsmerkmale Parität ist für v0.4.19 nicht erforderlich.
