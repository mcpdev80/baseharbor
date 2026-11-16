# ADR 0004: Pro-Anwendung mTLS geheime Broker

## Status

Akzeptiert für den MVP-Laufzeit-geheimen Pfad.

## Entscheidung

Jede BaseHarbor-Anwendung, die dynamische verwaltete Geheimnisse verwendet, erhält einen eigenen Runtime Secret Broker. Der Broker ist keine geteilte Multi-Tenant Runtime API.

Der Broker:

- an genau eine Anwendung und Umgebung gebunden ist;
- ist nur an das Backend-Netzwerk und das interne `baseharbor-secrets` Netz,
- hat keine Docker/Podman-Buchse;
- hat keine BaseHarbor-Manager- oder Root-Anmeldeinformationen;
- authentifiziert OpenBao nur mit der AppRolle dieser Anwendung;
- läuft schreibgeschützt, nicht-root, mit allen Linux-Funktionen fallen gelassen und `no-new-privileges`;
- verlangt von der internen CA BaseHarbor ausgestellte Kundenzertifikate;
- erfordert das Client-Zertifikat URI SAN `spiffe://baseharbor/apps/<app>/<environment>`;
- Zusätzlich ist der anwendungsskopierte Laufzeitträger-Token erforderlich;
- stellt keinen Host-Port für normalen Anwendungsverkehr frei.

Anwendungen erhalten die Broker-URL und mTLS-Dateistandorte als Laufzeitumgebungsvariablen. BaseHarbor erstellt und projiziert die Anmeldeinformationen automatisch; Anwendungen konfigurieren keine PKI-Topologie.

Der interne CA-Privatschlüssel wird niemals als hostseitige Schlüsseldatei beibehalten. BaseHarbor generiert/signiert im Speicher und speichert das CA-Material in einem nur für den Manager zugänglichen OpenBao-Namensraum. Leaf-Client-/Server-Schlüssel werden nur im geschützten Anwendungslaufzeitzustand gespeichert und nur in die Workloads eingebunden, die sie benötigen.

## Begründung

Ein gemeinsam genutzter Broker, der an mehrere Anwendungsnetzwerke angeschlossen ist, würde den Explosionsradius erhöhen. Per-Application Broker bewahren Netzwerk und geheime Domänenisolation, auch wenn ein Broker kompromittiert wird.

mTLS allein wird nicht als Autorisierung behandelt. Laufzeitzugriff ist geschichtet:

1. Netzisolierung;
2. CA-Validierung und anwendungsspezifische URI SAN;
3. Programmlaufzeit-Token;
4. anwendungsspezifische OpenBao AppRolle;
5. OpenBao-Richtlinie beschränkt auf den Anwendung geheimen Anwendungsbereich.

## Standalone-Kompatibilität

Anwendungen bleiben ohne BaseHarbor lauffähig. Der Broker ist eine optionale BaseHarbor Laufzeitfähigkeit und ersetzt nicht Standarddatenbank-, Redis-Protokoll-, HTTP-, TLS- oder Providerkonfigurationsverträge.
