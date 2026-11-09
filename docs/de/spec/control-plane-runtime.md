# Laufzeit des Control Planes

BaseHarbor stellt seine geschützte Steuerungs-Plane API durch die `baha serve` Prozess. Der Server ist bewusst von Anwendungsserviceprotokollen getrennt: PostgreSQL, Valkey, OpenBao und zukünftige S3-kompatible Dienste bleiben über ihre nativen Schnittstellen direkt konsumierbar.

## Gründungsvertrag

`baha serve` Fehler, die vor dem Öffnen des Hörers geschlossen werden, es sei denn, alle erforderlichen Laufzeitabhängigkeiten können konstruiert werden:

- die Steuerebene PostgreSQL DSN ist konfiguriert und erreichbar
- der konfigurierte OIDC-Emittent ist HTTPS und Entdeckung gelingt
- mindestens ein OIDC-Publikum konfiguriert ist
- das TLS-Zertifikat und der private Schlüssel sind vorhanden und bilden ein brauchbares Schlüsselpaar
- Authentifizierung, Mieterauflösung, RBAC, Applikationseigentum und anwendungsgeheime Handler können zusammengesetzt werden

Der Server fällt nie auf einen nicht authentifizierten oder Klartext-API-Hörer zurück, wenn eine dieser Anforderungen fehlt.

## Einrichtung

Die aktuelle Laufzeitkonfiguration ist explizite Prozesskonfiguration:

```text
BASEHARBOR_API_DATABASE_URL
BASEHARBOR_API_OIDC_ISSUER
BASEHARBOR_API_OIDC_AUDIENCES
BASEHARBOR_API_TLS_CERT_FILE
BASEHARBOR_API_TLS_KEY_FILE
BASEHARBOR_API_LISTEN_ADDR
```

`BASEHARBOR_API_OIDC_AUDIENCES ` ist eine Komma-getrennte Liste.`BASEHARBOR_API_LISTEN_ADDR ` Standardwerte für`127.0.0.1:8443` wenn sie weggelassen werden.

Geheime Datenbank-URLs müssen durch die Laufzeitumgebung oder eine andere vertrauenswürdige Prozessgrenze injiziert werden und dürfen nicht zur Quellkontrolle verpflichtet werden.

## HTTP-Grenzen

Die Laufzeit entlarvt:

```text
GET /healthz
/api/v1/...
```

`/healthz` ist ein nicht authentifizierter Endpunkt für die Prozesslebendigkeit und legt keine Konfigurations- oder Abhängigkeitsdetails offen.

Jede `/api/` Anfrage geht durch die obligatorische Sicherheitskette:

```text
TLS
  -> bearer token parsing
  -> OIDC discovery/JWKS-backed token verification
  -> identity-scoped membership resolution under PostgreSQL RLS
  -> tenant context
  -> RBAC
  -> authoritative application ownership
  -> protected application API
```

Die aktuelle geschützte API-Oberfläche enthält die anwendungsgeheimen Endpunkte. Geheimwerte werden niemals durch einen enthüllen Endpunkt freigelegt.

## Entwicklungsgateway im LAN (explizite Freigabe)

Eine reine Core-Installation kann den verwalteten Operator-Keycloak vor dem
ersten App-Deployment einrichten. Dazu wird die gemeinsam genutzte Gateway-
Route vor dem Abruf der OIDC-Discovery erzeugt.

Standardmäßig lauscht das Gateway nur auf `127.0.0.1`. Namen unter
`*.baha.localhost` oder `*.localhost` eignen sich nicht als Adresse für
andere Rechner.

Für einen ausdrücklich freigegebenen LAN-Zugriff:

1. Mit `baha dev domain DOMAIN` einen auf beiden Rechnern auflösbaren
   Ziel-Domainnamen konfigurieren. Insbesondere muss `auth.DOMAIN` von
   beiden Rechnern auf den Core-Host zeigen.
2. `BASEHARBOR_DEV_GATEWAY_BIND_ADDRESS` beim Aufruf
   `baha login --init-tenant` und bei späteren Gateway-Reconciles auf die
   konkrete, nicht lokale IPv4-Adresse des Core-Hosts setzen. Ein leerer
   Wert behält den bisherigen Loopback-Standard. Wildcard-Binds sind
   verboten.
3. Die echte verwaltete Core-CA über die vorgesehenen Operator- und
   Verbindungsprofile verifizieren. Keine TLS-Prüfung abschalten.
4. `baha serve` mit vorbereiteten, CA-signierten API- und Connector-
   Zertifikaten und korrekten SAN-Einträgen starten. Erst danach erstellt
   `node add` das Enrollment auf Basis der verifizierten
   `node-authority-profile.json`.

`baha serve` verifiziert den Operator-OIDC-Issuer über dieselbe verwaltete
Trust-Auflösung wie `baha login`: zuerst ein explizites
`BASEHARBOR_OPERATOR_OIDC_CA_FILE` (fehlend oder kein Zertifikat bricht ab),
dann die verwaltete CA, die der Bootstrap für den owned Core-Target geschrieben
hat, danach den zustandsweiten Standardpfad. Die verwaltete CA wird dem
System-Trust-Pool nur hinzugefügt; eine Installation in den System-Trust ist
nicht erforderlich und die Prüfung wird nie abgeschwächt.

Das Gateway-Zertifikat stammt aus der bereits vorhandenen verwalteten
OpenBao-Service-PKI. Die API- und Connector-Zertifikate werden normalerweise getrennt
bereitgestellt. Für eine ausdrücklich verwaltete LAN-Einrichtung kann
`baha serve` mit `BASEHARBOR_MANAGED_CORE_TLS=true` sowie
`BASEHARBOR_API_PUBLIC_URL`, `BASEHARBOR_CONNECTOR_PUBLIC_ADDRESS` und
passenden `BASEHARBOR_API_LISTEN_ADDR`- und
`BASEHARBOR_CONNECTOR_LISTEN_ADDR`-Werten beide Zertifikate aus der
**bereits READY gemeldeten** OpenBao-Service-PKI ausstellen. Das ist eine
explizite Dev-Option: keine neue CA, keine automatische Netzfreigabe,
kein ACME und kein allgemeiner PKI-Dienst. Die privaten Schlüssel verbleiben
im geschützten Core-Target unter `managed-serving/`. Fehlt in diesem expliziten Modus `BASEHARBOR_API_DATABASE_URL`, verwendet
die Operator-API die geschützte PostgreSQL-Verbindung und die verifizierte
Datenbank-CA ihrer lokalen Core-Installation. Zugangsdaten werden niemals an
Remote-Targets kopiert. Extern bereitgestellte Zertifikate werden niemals
überschrieben. Domainkonfiguration und Listener eröffnen keine zusätzlichen
Operator-, Tenant- oder Node-Rechte.

## Anwendungsbereich des TLS

Diese Laufzeit verbraucht ein bereits vorbereitetes Zertifikat/Schlüsselpaar. Zertifikatsausgabe, ACME, interne PKI, BYOC Entdeckung, Kettenbau und Erneuerung gehören zum dedizierten Zertifikatsmanagement-Meilenstein und werden hier absichtlich nicht umgesetzt.

Der Server benötigt TLS und verwendet TLS 1.2 oder neuer. Ein fehlendes oder ungültiges Zertifikat/Schlüsselpaar verhindert das Starten.

## Herunterfahren

Die wichtigsten `baha` Prozess wandelt SIGINT und SIGTERM in Kontextstornierung um.`baha serve` führt dann eine begrenzte anmutige HTTP-Abschaltung vor dem Schließen seines PostgreSQL-Pools durch.

## Zuständigkeitsgrenze (Server vs. lokale Ausführung)

Der Control-Plane-Server ist eine Remote-Grenze. Sie adressiert die ausgewählte
Installation-Autorität, die maßeblichen Deployment-, Anbieter-, Identitäts-,
Consent-Revision- und Mutationszustand für seine admitted Environment verwaltet.

Die lokale Direktausführung ist unabhängig davon, ob der Server präsent oder
erreichbar ist: sie wiederverwendet denselben semantischen Kern
(`internal/coreinstallation`, `internal/deployment`), ohne die OIDC/PostgreSQL-
Operatorgrenze zu verpflichtet, die die Remote-API verlangt. Ein Remote-Client,
der die ausgewählte Installation nicht durch seine Managerverbindung erreichen
kann, schließt fehl; sie ersetzt niemals lokale Ausführung für eine fehlende,
fremde oder abgelaufene Remote-Autorität, und eine gecachte last-known-Zustand
ist eine veraltete `observed-at`-Beobachtung, niemals aktuelle „READY".

Siehe [ADR 0021](../decisions/0021-one-installation-authority.md).
