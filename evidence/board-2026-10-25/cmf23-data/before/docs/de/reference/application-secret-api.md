# Anwendung Secret Service und API-Grenze

Die geheimen Operationen der BaseHarbor-Anwendung haben eine vertrauenswürdige Orchestrierungsgrenze:`internal/applicationsecret.Service`.

Das `baha app secret` Befehle und der HTTP-Anwendung-geheime Handler verwenden diesen Dienst, anstatt unabhängige OpenBao-Workflows zu implementieren. Dies hält Validierung, Laufzeitauflösung, Provider-Zugriff, Timeouts und geheime Metadaten-Semantik an einem Ort.

## HTTP-Vertrag

Der aktuelle Handler definiert diese BaseHarbor-spezifischen Operationen:

```text
GET    /api/v1/apps/{app}/secrets
PUT    /api/v1/apps/{app}/secrets/{name}
DELETE /api/v1/apps/{app}/secrets/{name}
```

`GET` liefert nur Metadaten zurück:

```json
{
  "secrets": [
    {
      "name": "OPENAI_API_KEY",
      "required": true,
      "present": true,
      "usable": true
    }
  ]
}
```

Es gibt absichtlich keine API-Operation, die einen gespeicherten geheimen Wert aufzeigt.

`PUT` akzeptiert einen neuen oder Ersatzwert:

```json
{
  "value": "..."
}
```

Eine erfolgreiche Antwort bestätigt nur den Schlüsselnamen und den konfigurierten Zustand. Der angegebene Wert wird nicht widergespiegelt.

`DELETE` entfernt das verwaltete Geheimnis dauerhaft durch denselben anwendungsgeheimen Dienst, der vom CLI genutzt wird.

## Genehmigungsgrenze

Der HTTP-Handler ist fehlgeschlagen und erfordert alle folgenden Schritte, bevor eine geheime Operation den Dienst erreichen kann:

1. eine authentifizierte `identity.Principal` im Rahmen des Ersuchens
2. ein gelöstes `tenancy.Context`
3. eine RBAC-Rolle, die die beantragte Lese-/Aktualisierungs-/Löschengenehmigung erteilt
4. positives Anwendungseigentumsnachweis aus einem `OwnershipResolver`

Fehlendes, mehrdeutiges oder negatives Eigentum fällt nie auf den Zugriff auf den Anwendungsnamen zurück.

Die Kenntnis eines Anwendungsnamens reicht daher nicht aus, um seine geheimen Metadaten zu inspizieren oder zu mutieren.

## Autoritatives Anwendungseigentum

Die Anwendung auf die Tenant-Eigenschaft wird in PostgreSQL in `application_ownerships`.

Die Invariante ist absichtlich klein und streng:

```text
application_name  -> exactly one tenant_id
```

`application_name ` ist der primäre Schlüssel und`tenant_id ` Literaturhinweise`tenants(id)`. PostgreSQL row-level security Scopes liest und schreibt in den aktuellen BaseHarbor Mieter-Kontext.

`ApplicationOwnershipStore.Claim` ist idempotent für den gleichen Mieter und Antrag. Ein anderer Mieter kann keinen bereits im Besitz befindlichen Anwendungsnamen beanspruchen. Eigentum wird niemals implizit oder mit Last-Write-Wins-Verhalten neu zugewiesen.

`ApplicationOwnershipStore.OwnedByTenant ` führt das Lesen innerhalb der bestehenden Pächter-Scope-Transaktionsgrenze aus, so dass der HTTP-Handler den datenbankgestützten Store direkt als dessen`OwnershipResolver`.

## Geschützte Anfragekette

Geschützte Steuerungs-Plane-Handler sind hinter einer obligatorischen Anforderungs-Sicherheits-Kette zusammengesetzt:

```text
Authorization: Bearer <token>
        ↓
OIDC discovery / JWKS verification
        ↓
identity.Principal
        ↓
identity-scoped TenantResolver
        ↓
tenancy.Context
        ↓
RBAC
        ↓
ApplicationOwnershipStore
        ↓
application secret handler
```

`auth.OIDCVerifier ` verwendet die etablierte`go-oidc` Umsetzung für Signatur, Emittenten, Ablauf und Publikumsvalidierung, anstatt JWT-Kryptographie innerhalb von BaseHarbor umzusetzen.

`database.IdentityTenantResolver ` löst Mitgliedschaften, bevor ein Mieter-Kontext über eine dedizierte PostgreSQL existiert`FOR SELECT ` RLS-Policy, die vom bereits geprüften Emittenten und Subjekt Keyed ist. Es nutzt keine Superuser-Verbindung oder eine Rolle mit`BYPASSRLS`.

Der Resolver scheitert geschlossen, wenn es keine Mitgliedschaft, ein ungültiger Auftraggeber oder Mitgliedschaften, die mehr als ein Mieter. Mehrere Rollen innerhalb des gleichen Mieters werden durch die bestehende gelöst `tenancy.Resolve`. . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . .

Siehe[Authentication](../explanation/authentication.md)für die vollständige Vertrauensbeschreibung.

## Aktueller Expositionsstatus

Der geschützte Handler, der Beton-OIDC-Verifier, der datenbankgestützte Applikationsbesitz und der sichere Mieter-Resolver sind implementiert und testbar.

BaseHarbor behauptet immer noch **nicht**, dass diese API öffentlich zuhört. Laufzeit-OIDC-Einstellungen, Datenbankverkabelung, Listener/TLS-Konfiguration und Startverifizierung müssen noch explizit vor dem Öffnen des Endpunkts des Control-Plane-Netzwerks zusammengestellt werden.

## Geheime Nichtoffenlegung

Geheime Werte dürfen nie in erscheinen:

- GET-Antworten
- erfolgreiche PUT-Antworten
- API-Fehler
- Metadaten der Anwendung
- normale Protokolle oder Telemetrie
- Prüfung der Nutzlasten

Die API-Tests beinhalten negative Autorisierungsfälle und überprüfen, ob ein eingereichter geheimer Wert in der Antwort nicht widergespiegelt wird.

## Laufzeit-Lieferung bleibt getrennt

Diese API verwaltet geheime Werte. Sie definiert nicht, wie ein Workload sie verbraucht.

Die Laufzeitbereitstellung bleibt ein Problem des Anbieters und kann später Standardmechanismen wie z.B.:

- In-Memory geheime Dateien
- gegebenenfalls explizite Umwelteinspritzung
- OpenBao/Vault-kompatible Workload-Identität
- Kubernetes-native Geheimprojektion

Der Bewerbungsvertrag erklärt weiterhin **was** erforderlich ist, nicht einen obligatorischen BaseHarbor-spezifischen Liefermechanismus.
