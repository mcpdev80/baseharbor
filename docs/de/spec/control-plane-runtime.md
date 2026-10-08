# Laufzeit des Steuerflugzeugs

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

## Anwendungsbereich des TLS

Diese Laufzeit verbraucht ein bereits vorbereitetes Zertifikat/Schlüsselpaar. Zertifikatsausgabe, ACME, interne PKI, BYOC Entdeckung, Kettenbau und Erneuerung gehören zum dedizierten Zertifikatsmanagement-Meilenstein und werden hier absichtlich nicht umgesetzt.

Der Server benötigt TLS und verwendet TLS 1.2 oder neuer. Ein fehlendes oder ungültiges Zertifikat/Schlüsselpaar verhindert das Starten.

## Herunterfahren

Die wichtigsten `baha` Prozess wandelt SIGINT und SIGTERM in Kontextstornierung um.`baha serve` führt dann eine begrenzte anmutige HTTP-Abschaltung vor dem Schließen seines PostgreSQL-Pools durch.
