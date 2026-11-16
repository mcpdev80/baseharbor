# Managed Service Access Inventory

Status: normatives Inventar für die aktuelle v0.4.17 Referenzlaufzeit komponieren.

Dieses Inventar erfasst die tatsächliche verwaltete Netzwerkservice-Grenze hinter Ausgabe #397. Es beschreibt nur das Bereitstellungs-/Laufzeitverhalten. Keines dieser Provider-Produkte oder konkrete Sicherheitsmechanismen wird zu tragbaren Anwendungsabsichten.

## Umweltpolitik

• Umwelt • Zugang für Menschen/Entwickler • Arbeitsbelastung/Dienstleistung
| --- | --- | --- | --- |
Der lokale/loopback-Zugriff ist Null-Zeremonie und nein `baha login` wird benötigt. native Anmeldeinformationen oder automatisch projizierte Service-Bindungen; mTLS bleibt dort verfügbar, wo die Laufzeit bereits verwendet wird.
Test-Voraussetzungen für BaseHarbor-Anwendungsoperationen erfordern einen authentifizierten OIDC-Operator für Ziel/Umwelt; Management-UIs bleiben standardmäßig eingeschränkt/nicht öffentlich , geschützte generierte Anmeldeinformationen, mTLS oder ein anderer konfigurierter Service-Access-Mechanismus .
Die Anwendung von BaseHarbor erfordert einen authentifizierten OIDC-Operator für Ziel/Umwelt; kein anonymer Management/Beobachtbarkeitszugriff ist obligatorisch; native Authentizität, mTLS, Scoped Token oder ein konfigurierter externer Adapter können die Richtlinien erfüllen.

Für freigegebene HTTP-Provider wird der Service-Access-Zustand nie stillschweigend von einer authentifizierungspflichtigen Umgebung zurück auf anonymen Zugriff herabgestuft, wenn eine spätere Reconciliation nur Entwicklungsverbraucher sieht.

Loopback und interne Netzwerke sind Verteidigung in der Tiefe. Sie werden nicht als Authentifizierung behandelt.

## Laufende verwaltete Dienstleistungen

Benannter Dienst Belichtung Transport Beglaubigung / Autorisierung Beglaubigung / Trust Ownership Beglaubigungsgrenze
| --- | --- | --- | --- | --- | --- |
. Control-Plane PostgreSQL . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . .`pg_hba.conf` erfordert TLS und SCRAM-SHA-256-generierten BaseHarbor-geschützten Zustand; Zertifikat Lebenszyklus durch den ausgewählten Service-Access-Emittent.
• Anwendung PostgreSQL® host loopback plus Anwendung Backend-Netzwerk • native PostgreSQL TLS® generierte pro-Anwendung Datenbankinformationen • generierte BaseHarbor-Anwendung Laufzeitzustand; CA projiziert durch sichere Dateibindung • PostgreSQL native TLS/auth
• Valkey / Redis-kompatibles Cache • Application Backend Netzwerk; Host-Zugriff über Loopback TLS Gateway • TLS Gateway vor dem nativen Valkey Service • pro Anwendung erzeugt `requirepass` Credential , generiert BaseHarbor Anwendung Laufzeitzustand; CA durch sichere Dateibindung projiziert , Valkey natives Passwort + geteilte TCP TLS Gateway ,
• OpenBao control-plane network plus host loopback; optionale Web-UI verwendet die gleiche native HTTPS/TLS , OpenBao native Authentifizierung; Manager-Operationen verwenden am wenigsten privileg AppRolle-Richtlinie , OpenBao besitzt verwalteten lokalen Emittenten Staat und CA privaten Schlüssel; Manager-Anmeldeinformationen sind geschützt Zustand , OpenBao 2.7 native TLS / Auth mit `tls_auto_reload` für Zertifikatsersatz
• SeaweedFS S3® Provider-Netzwerk plus Host-Loopback; optionaler geteilter Admin-UI bleibt ein separater HTTPS-Adapter; • nativer S3 HTTPS® nativer S3 Zugriffsschlüssel/geheim für Anwendungen; • Admin-UI behält seine dedizierte Browser-Zugriffsrichtlinie; • provider-admin-Anmeldeinformationen bleiben innerhalb des Provider-Zustands; • Anwendungsanmeldeinformationen sind scoped geschützte Bindungen; • SeaweedFS native TLS/S3 Auth; • nur die Admin-UI behält einen Adapter; •
• Managed Keycloak identity • Provider-Netzwerke plus ein nativer Loopback HTTPS Hörer; kanonische dev login/admin Hosts Route durch das Target gateway • native Keycloak HTTPS • Standard OIDC / OAuth2 für Anwendungen/Benutzer; • Keycloak-native Admin-Authentifizierung bleibt Provider-Admin-Admin-Admin-Route durch das Target-Gateway • Application-Client-Secret ist ein geschützter Anwendungs-Bindungszustand; • Provider-Admin-Anmeldeinformationen bleiben Provider-Zustand • Keycloak native TLS / OIDC • Zertifikate laden alle 30er Jahre neu; • Target gateway liefert kanonisches Browser-Routing •
PgAdmin companer • Anwendungslaufzeit; nur Host-Loopback • natives HTTPS • pgAdmin-natives Login • verwaltete PostgreSQL-Verbindungen werden über einen geschützten Laufzeitzustand vorkonfiguriert • UI-Login und Datenbank-Anmeldeinformationen • BaseHarbor-Anwendungslaufzeitzustand • sind nicht anwendungsbezogene optionale UI-Verwaltung •
Redis Commander companer; Host-Loopback über HTTPS-Proxy ® HTTPS/TLS-Gateway + HTTP Basic ® generierte UI Basic Auth plus app-scoped cache credential hinter dem UI ® UI und Cache-Anmeldeinformationen bleiben BaseHarbor-Anwendungs-Laufzeitzustand ® app-scoped optionale UI
Prometheus® Provider/interne Netzwerke; natives HTTPS auf Host-Loopback- und kanonische Dev-Route durch das Target-Gateway; natives Prometheus HTTPS® dev verwendet natives Basic Auth, wenn Management-Anmeldeinformationen konfiguriert werden; verwaltete Test-/Prod-Nutzung nativen mTLS-Zertifikatlebenszyklus durch ausgewählten Emittenten; Web-TLS-Konfiguration wird auf jeder Anfrage erneut gelesen.
Loki-interne/provider-Netzwerke; Entwicklerendpunkt auf Host-Loopback-Versionen; HTTPS/TLS-Gateway; gleiche Richtlinie für den umgebungsbewussten Zugang zu Diensten wie Prometheus; Registrierung/Erfassung von Anwendungen bleibt kontrolliert; Zertifikats-Lebenszyklus durch ausgewählten Emittenten; Sammler-/Anbieterstaat bleibt BaseHarbor-eigenes shared HTTP-Service-Access-Gateway plus Loki/Alloy-Registrierungs-Grenze
Das Netzwerk des Entwicklers auf dem Host-Loopback. HTTPS/TLS-Gateway. environment-aware service-access auth; mTLS ist der aktuelle verwaltete Standard, bei dem die Authentifizierung erforderlich ist. Zertifikatslebenszyklus durch ausgewählte Emittent. . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . .
OpenTelemetry Collector / OTLP-Telemetrie-Netzwerk; Entwickler/Testendpunkt auf Host-Loopback, wenn man es verwaltet; nativer Sammler HTTPS-Environment-aware-Service-Access-Auth; verwalteter Test/Prod-Einsatz nativer mTLS; Workload CA/Client-Material wird automatisch projiziert; Zertifikat/Client-Material ist geschützt Laufzeitbindung, nicht Anwendungsabsicht; Kollektor lädt Zertifikate alle 30er Jahre neu; OpenTelemetry-Empfänger native TLS/mTLS-Empfänger
Anwendung Laufzeit Broker-Anwendung Backend / Control-Netzwerke; docs listener loopback nur native HTTPS-Scope-basierte mTLS-Identität plus geschützte Laufzeit Träger / Service Tokens Blatt-Identitäten durch verwalteten Emittenten ausgegeben; App Token und Berechtigungen sind anwendungsscoped Protected State.
• Laufzeit-Provider-Executor `baseharbor-runtime-control` Nur Netzwerk; kein Host-veröffentlichter API-Port . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . .
Verwaltete HTTP-Belichtung Provider/Anwendungs-Belichtungsnetzwerk und konfigurierter Hörer HTTPS/TLS nach Bereitstellung TLS-Status ® anwendungsorientierte Auth bleibt anwendungs-/providerverantwortlich, es sei denn, eine spätere Identity/Auth-Fähigkeit wird ausgewählt ® current public ingress certificate lifecycle rests separate deployment TLS state ® exposure provider ®
Grafana, derzeit nicht instantiiert durch die v0.4.17 Referenzlaufzeit, n/a. n/a. n/a. n/a.

## PKI / Trust Source Modelle

Die gleiche Service-Access-Grenze akzeptiert:

- `managed-local`: derzeitiger Referenzemittent ist OpenBao PKI;
- `external-pki`: statisches externes Material oder eine von einem Emittenten unterstützte Integration, die durch eine anbieterneutrale Emittentsreferenz gekennzeichnet ist;
- `byoc`: im Eigentum des statischen Bedieners befindliches Zertifikat/Schlüssel/Vertrauensmaterial;
- externe Vertrauensbündel;
- künftige Runtime/Plattform-Emittenten, einschließlich Incluster Kubernetes/OpenShift-Implementierungen.

Die Änderung der Treuhand-/Emittentenrealisierung ändert nicht die portable Anwendungsabsicht.

## Verantwortung für den Lebenszyklus des Zertifikats

Quelle: Eigentümer, Erneuerungsmodus, Basis-Harbor-Verhalten
| --- | --- | --- | --- |
Gemanagt-lokal , ausgewählte Emittent Anbieter , automatische Abstimmung , inspizieren Ablauf , Erneuerung durch `Issuer.Renew`, atomar Projekt Ersatzmaterial und lassen Sie normale Anbieter / Laufzeit Abgleich rollen es aus
External-pki mit Emittent-Adapter, externe Emittent-Integration, automatische Abstimmung, wenn unterstützt, erfordern exakte Emittent-Referenz-Match, Anfrage Verlängerung durch die gleiche `Issuer` Vertrag und Erhaltung des Arbeitsaufwands Bindung Form
External-pki Statische Dateien External/Operator - Ersetzen und Versöhnen Validierung Material und Oberflächen-Auslauf Zustand; BaseHarbor beansprucht nicht Emittent Eigentum
Beschreiben Sie, dass der Betreiber die Schlüssel/Zertifikate/Vertrauen validieren, vor Ablauf warnen und atomar Ersatzmaterial auf die Versöhnung verbrauchen wird.

Die dem Status/Doktor exponierte Lebenszyklusbeobachtung ist geheim und umfasst nur Quelle, Eigentümer, Erneuerungsmodus, Emittentenreferenz, Ablauf und Gesundheit. Sie enthält niemals private Schlüssel, Träger-Token oder Credential-Werte.

## Begrenzung der Arbeitslastprojektion

Verknüpfung Metadaten und Dateireferenzen können in Workload-Umgebungsvariablen projiziert werden. Zertifikat und Schlüsselinhalte sind es nicht.

Beispiele:

```text
DATABASE_URL=postgresql://...
DATABASE_CA_FILE=/run/baseharbor/bindings/postgres/default/ca.pem

REDIS_URL=rediss://...
REDIS_CA_FILE=/run/baseharbor/bindings/valkey/default/ca.pem

AWS_CA_BUNDLE=/run/baseharbor/tls/s3/ca.pem
OTEL_EXPORTER_OTLP_CERTIFICATE=/run/baseharbor/tls/otlp/ca.pem
```

Das referenzierte CA/certificate/key-Material wird als schreibgeschützte oder geschützte Geheimdateien eingehängt. Dies ist die stabile Grenze, die eine zukünftige Kubernetes/OpenShift-Laufzeit durch Secret/ConfigMap/CSI-Stilprojektionen ohne Änderung der Anwendungsabsicht realisieren kann.


## Native-TLS-erste Topologieregel

Die Referenzlaufzeit v0.4.17 folgt dieser Reihenfolge:

1. die Diensteanbieter-native TLS verwenden, wenn der Anbieter die erforderliche Transport- und Authentifizierungspolitik durchsetzen kann;
2. das einzelne Target-scoped Developer Gateway für das kanonische Browser-Routing in der Entwicklung verwenden;
3. einen dedizierten Adapter nur dann beibehalten, wenn er eine Sicherheits- oder Protokolleigenschaft hinzufügt, die der native Endpunkt nicht bereitstellt.

Aktuelle gerechtfertigte Adapter sind:

- Valkey TCP-Zugriff, der TLS um das passwort-authentifizierte Valkey-Protokoll ergänzt;
- Redis Commander/Cache Management UI, die keinen gleichwertigen nativen HTTPS-Hörer in der ausgewählten Komponente hat;
- SeaweedFS Admin UI, während S3 selbst native HTTPS verwendet;
- Loki- und Tempo-Host/API-Zugriff, da ihre aktuelle BaseHarbor-Topologie auch interne Legierungs-/Sammler-Ingestions- und Provider-Scrape-Beziehungen enthält, die absichtlich auf einem klaren internen Transport isoliert bleiben; die Umwandlung dieser Kanten erfordert eine koordinierte clientseitige TLS-Migration, anstatt lediglich einen Proxy zu löschen;
- verwaltete Anwendungsexposition, wenn der Proxy der tatsächliche Ingress/Expositionsanbieter ist und nicht ein Service-Access-TLS-Wrapper.

Keycloak, OpenBao, PostgreSQL, SeaweedFS S3, Prometheus und OTLP benötigen keinen speziellen Caddy-Container mehr, um TLS zu erhalten.
