# Managed Service Access und PKI

Von BaseHarbor verwaltete Netzwerkdienste verwenden TLS oder einen stärker authentifizierten Transport.

Zertifikatsausgabe und Trust Ownership sind bewusst getrennt von portablen Anwendungsabsichten. BaseHarbor benötigt keine eigene CA.

## PKI-Quellen

Die Service-Access-Schicht unterstützt:

- `managed-local`— BaseHarbor löst die Zertifikatsausgabe über die anbieterneutrale Emittentsgrenze; die aktuelle Compose-Referenzrealisierung ist OpenBao PKI;
- `external-pki `— entweder statisch extern ausgegebenes X.509-Material oder ein Anbieter-neutraler externer Emittent-Adapter ausgewählt durch` BASEHARBOR_SERVICE_PKI_ISSUER_REF`;
- `byoc`— ein Betreiber Bescheinigungs-/Schlüssel-/Vertrauensakten ohne Übertragung der Emission oder des CA-Eigentums an BaseHarbor liefert.

Provider-spezifische Operator-Eingänge überschreiben die generischen Service-Standards. Zum Beispiel,`BASEHARBOR_PROMETHEUS_PKI_SOURCE ` Überbrückungen`BASEHARBOR_SERVICE_PKI_SOURCE`.

Die generischen Operatorvariablen sind:

```text
BASEHARBOR_SERVICE_PKI_SOURCE
BASEHARBOR_SERVICE_TLS_CERT_FILE
BASEHARBOR_SERVICE_TLS_KEY_FILE
BASEHARBOR_SERVICE_TLS_TRUST_FILE
BASEHARBOR_SERVICE_TLS_CLIENT_CERT_FILE
BASEHARBOR_SERVICE_TLS_CLIENT_KEY_FILE
BASEHARBOR_SERVICE_TLS_SERVER_NAME
BASEHARBOR_SERVICE_PKI_ISSUER_REF
```

Anbieterspezifische Formulare ersetzen `SERVICE` mit der Großraum-Anbieter-Kennung, z.B.`BASEHARBOR_LOKI_TLS_CERT_FILE`.

Diese Werte sind Deployment/Operator Zustand, nicht Felder in `baseharbor.yaml`.

## Umweltvorschriften

- dev: TLS ist obligatorisch; vertrauenswürdiger lokaler Zugriff kann Authentifizierungslicht bleiben, wenn die Provideroberfläche nur Loopback/intern ist.
- Test: TLS und automatierbare Authentifizierung sind verpflichtend.
- prod und benutzerdefinierte verwaltete Umgebungen: TLS und Authentifizierung sind für Management/Beobachtungs-/Kontrolloberflächen verbindlich.

Menschliche Management-Oberflächen folgen zusätzlich[Management surface access v1](management-access-v1.md): natives OIDC/OAuth2 zuerst, dann ein vorhandener Standard-basierter Adapter, dann provider-native Anmeldeinformationen, ansonsten explizit nicht unterstützt. BaseHarbor erstellt dafür keinen proprietären Auth-Proxy.

## Verzeichnis verwalteter Netzwerkdienste

Das folgende Inventar beschreibt die aktuelle Compose-Umsetzung. Es ist ein Beleg für das aktuelle Provider/Laufzeit-Mapping, nicht für portable Anwendungsabsichten.

Belichtung auf der Oberfläche, Verkehr, Authentisierung, Arbeitsbelastung/Kundenvertrauen
| --- | --- | --- | --- | --- |
• OpenBao UI/API host loopback plus control-plane network • natives HTTPS • OpenBao natives Token / AppRole semantics bleiben durch die Service-Access-Emittentengrenze autoritativ • Managed/External CA
Der Host-Loopback von PostgreSQL ist nur nach der Bereitschaft des Emittenten verfügbar, native PostgreSQL TLS; Klartext TCP wird abgelehnt von `hostnossl` policy---PostgreSQL SCRAM/native Anmeldeinformationen------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
• Anwendung PostgreSQL ® Anwendung Backend Netzwerk plus Loopback Entwickler port ® native PostgreSQL TLS; Klartext TCP abgelehnt von `hostnossl` policy-scoped PostgreSQL-Anmeldeinformationen `DATABASE_URL`+ nur lesbar ` DATABASE_CA_FILE`Bindemittel
Anwendung Valkey/Redis-Anwendungs-Backend-Netzwerk plus Loopback-Entwickler-Port durch Zugriffs-Gateway.... Valkey-Passwort/native Authentifizierung....`rediss://` URL + schreibgeschützt `REDIS_CA_FILE` / ` VALKEY_CA_FILE`Bindemittel
Prometheus-Providernetzwerke plus Loopback native HTTPS; kanonische dev-URL über das Target-Gateway. natives HTTPS. natives Basic Auth für konfigurierten dev-Management-Zugriff; natives mTLS für verwalteten Test/prod. CA/Client-Identität, projiziert durch Service-Zugriffsstatus.
Loopback HTTPS API-Gateway: Loopback HTTPS HTTPS® gleiche umgebungsbewusste Richtliniengrenze wie andere Beobachtbarkeitsoberflächen: CA/Client-Identität, projiziert durch Service-Access-Zustand.
Tempo-internes Netzwerk; Loopback HTTPS API-Gateway. HTTPS. gleiche umweltbewusste Richtliniengrenze wie andere Beobachtbarkeitsoberflächen. CA/Client-Identität, projiziert durch Service-Access-Zustand.
OpenTelemetrie-Kollektor-Telemetrie-Netzwerk plus loopback native HTTPS, wo aktiviert ist, native HTTPS. Managed test/prod verwenden native mTLS; externe Endpunkte müssen HTTPS sein.`OTEL_EXPORTER_OTLP_CERTIFICATE` und optionale Client cert/key Dateibindungen
Meeresalgen-FS/S3-Provider-Netzwerk plus Loopback nativen HTTPS. nativen HTTPS. S3/native Anmeldeinformationen `AWS_CA_BUNDLE`/ S3 CA Datei-Bindung
• Runtime Broker / Executor • interne Runtime-Control-Netzwerke; keine generische öffentliche Provider-Admin-Oberfläche • mTLS plus bestehende Broker-Token/Identitätssemantik • bestehende Runtime Broker • SPIFFE / Token Autorisierung • Laufzeit • CA/Client Zertifikat / Key File Bindungen •

### Netzwerkplatzierung ist keine Authentifizierung

Loopback-Bindung, interne Compose-Netzwerke, Kubernetes-Namensräume und gleichwertige Laufzeitplatzierung sind defensive-in-depth-Steuerungen. Sie reduzieren die Erreichbarkeit, dürfen aber NICHT als Authentifizierung behandelt werden.

Für verwaltete Test-/Prod-Oberflächen bleibt der gewählte Authentifizierungsmechanismus auch dann verbindlich, wenn der Endpunkt nur über Loopback oder ein internes Netzwerk erreichbar ist. Provider-native Anmeldeinformationen, mTLS, Token-Authentifizierung, OIDC/OAuth2 oder ein zukünftiger Managed-Identity-Adapter können dieser Anforderung gemäß der Richtlinie genügen.

Die gleiche Service-Access-Richtlinie ist runtime-neutral. Docker/Compose und Podman-Realisierungen verbrauchen die gleichen aufgelösten TLS/Authentifikations-Semantiken; zukünftige Kubernetes/OpenShift-Realisierungen übersetzen die gleiche Provider-neutrale Bindung in native Secret/ConfigMap/CSI/Service-Konstrukte anstatt die Applikations-Intention zu ändern.

## Entwickler-Host-Trust

Managed-local PKI kann seine öffentliche CA dem Entwicklerhost aussetzen, ohne Emittentenbesitz an die CLI zu übertragen.

Regeln:

- `baha up` prüft, ob der Host dem aktiven verwalteten lokalen CA bereits vertraut, wenn der Lebenszyklus des Repositorys einen initialisierten, nicht versiegelten Emittenten erreicht;
- interaktiv `baha up` kann Host-Trust-Installation anbieten, jedoch nur nach einem ausdrücklichen Ja/Nein-Prompt;
- `--yes` niemals impliziert die Zustimmung des Gastgeber-Vertrauens;
- Automatisierung entscheidet sich explizit mit `baha up --trust-host-ca` or ` baha trust install --yes`;
- `baha trust export --output PATH` Ausfuhr nur das öffentliche CA-Zertifikat/-Bundle; die privaten CA-Schlüssel verbleiben innerhalb des Emittentenanbieters;
- BaseHarbor erfasst nur Vertrauensanker, die es selbst installiert hat, mit CA-Fingerabdruck und Emittent-Referenz-, Backend- und Ankerpfad;
- gewöhnlich `baha down` wahrt das Vertrauen der Gastgeber, weil die Umwelt noch vorhanden ist;
- global `baha destroy --yes` entfernt nur BaseHarbor-eigene Host-Trust-Anker und überprüft den installierten Zertifikat-Fingerabdruck vor der Löschung;
- von einem Betreiber oder einem anderen Werkzeug bereits installierte Vertrauenswurzeln werden als vertrauenswürdig erkannt, werden aber nie als BaseHarbor-Eigentümer beansprucht;
- Externe-pki- und BYOC-Vertrauenswurzeln bleiben Betreiber-Eigentümer und werden niemals als BaseHarbor-Eigenmaterial exportiert oder von BaseHarbor entfernt.

Fingerabdruck-spezifische Ankernamen ermöglichen es alten und neuen Wurzeln, sich während der CA-Rotation zu überschneiden, anstatt destruktiven Ersatz an Ort und Stelle zu erzwingen.

## Zertifikat Lebenszyklus und externe PKI

Der Zertifikatslebenszyklus bleibt hinter der Anbieter-/Laufzeitgrenze zurück und ändert niemals portable Anwendungsabsichten oder Workload-Bindungsnamen.

### Örtlich verwaltet

Managed-local-Blatt-Zertifikate sind kurzlebig und versöhnt vor Ablauf. Das aktuelle Erneuerungsfenster beträgt sieben Tage.

Der Service-Access-Zustand erfasst nur nicht-geheime Lebenszyklus-Metadaten:

```text
issuer_reference
lifecycle_owner=issuer
renewal_mode=automatic-reconcile
server_serial
server_expires_at
client_serial
client_expires_at
```

Ein stabiles Zertifikat außerhalb des Erneuerungsfensters bleibt unberührt.`Issuer.Renew()`. Ein veränderter Emittent vertraut root zwingt Ersatzausgabe auch dann, wenn das alte Blatt sonst noch gültig ist. Der umgebende Anbieter/Laufzeit versöhnt Projekte der Ersatz atomar und startet/versöhnt den betroffenen Service über seinen bestehenden Lebenszyklusweg.

Das vorige Blatt wird nicht vor dem Ersatz-Rollout widerrufen. Dadurch wird ein Ausfallfenster vermieden; es kann natürlich auslaufen oder durch einen späteren Anbieter-spezifischen verifizierten Rollout-Hook widerrufen werden.

### Externes PKI

Externes PKI ist erstklassig und verfügt über zwei Bereitstellungsmodi:

1. **statisches externes Material** — Zertifikat/Schlüssel/Trust-Dateien bleiben extern im Eigentum und werden von BaseHarbor validiert/vorgesehen;
2. **externes PKI**, mit einer Ausgabe versehen `BASEHARBOR_SERVICE_PKI_ISSUER_REF` wählt einen Anbieter-neutralen Emittent-Adapter aus.

Emittenten unterstützt externe PKI verwendet die gleichen `Issuer` Vertrag als Managed-local PKI. Der Adapter muss die genaue Emittentsreferenz melden, die von der Richtlinie verlangt wird; ein falsch abgestimmter Adapter scheitert geschlossen. Dies verhindert, dass ein OpenBao/Default-Emittent versehentlich eine Enterprise-PKI-Richtlinie erfüllt.

Statisch `external-pki` und `byoc` Nachweise über den geheimen Lebenszyklus im geschützten Zustand des Zugangs zum Dienst:

```text
source
lifecycle_owner
renewal_mode=replace-and-reconcile
server_fingerprint
server_expires_at
client_expires_at
health
warning
```

Ablauf innerhalb von 30 Tagen ist `warn`; Ablauf innerhalb von sieben Tagen ` critical`. Der Ersatz verwendet die gleichen konfigurierten Quellpfade: Nachdem der Operator/Provider die Zertifikat/Schlüsseldateien ersetzt, bestätigt die nächste Abstimmung das neue Paar, aktualisiert den Fingerabdruck/Auslaufnachweis und rollt ihn durch die gleichen Arbeits-/Providerbindungspfade.

`byoc` ist immer Betreiber-Eigentum und kann keinen Emittent-Adapter deklarieren. BaseHarbor erhebt niemals Emissionsrechte für BYOC-Material.

Private Schlüssel bleiben geschützter Zustand. Normaler Status/Evidenz-Ausgang zeichnet nur Quelle/Eigentum/Fingerabdruck/Ausfall-Metadaten auf, nie Schlüsselmaterial oder kreidentielle URLs.

Der aktuelle Managed-local Referenzemittent ist OpenBao PKI. An der gleichen Grenze schließen zukünftige Emittentsintegrationen (z.B. Enterprise Certificate Services, Cert-Manager oder OpenShift Emittents) an. Ein Emittentsprodukt darf niemals zu tragbaren Anwendungsabsichten werden.
