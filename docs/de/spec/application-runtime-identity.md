# Identität der Laufzeit der Anwendung

Anwendungen benötigen eine nicht-menschliche Identität, wenn sie den BaseHarbor Application Runtime Broker zur Laufzeit verwenden. Verwaltete OpenBao-Geheimnisse sind die erste Produktionsnutzung dieser Identität; Laufzeitressourcen und asynchrone Funktionsoperationen verwenden die gleiche Grenze.

Die Laufzeitidentität ist die von BaseHarbor bereitgestellte Infrastruktur. Sie ist keine Anwendungs-Business-Konfiguration und fügt daher keinen generischen Runtime-API-Umstieg zu `baseharbor.yaml`.

## Vertrag über die Arbeitslast

Für eine Projektarchiv-Workload, die den Application Runtime Broker erfordert, injiziert BaseHarbor:

```text
BASEHARBOR_RUNTIME_API_URL=https://baseharbor-runtime:8443
BASEHARBOR_RUNTIME_TOKEN_FILE=/run/secrets/baseharbor-runtime-token
BASEHARBOR_RUNTIME_CA_FILE=/run/secrets/baseharbor-runtime-ca
BASEHARBOR_RUNTIME_CLIENT_CERT_FILE=/run/secrets/baseharbor-runtime-client-cert
BASEHARBOR_RUNTIME_CLIENT_KEY_FILE=/run/secrets/baseharbor-runtime-client-key
```

Anwendungen verwenden gewöhnliches HTTPS mit dem mitgelieferten mTLS-Identitäts- und App-Scope-Laufzeit-Token. Es ist kein obligatorisches BaseHarbor SDK erforderlich.

Der Broker akzeptiert nur die Anwendung Identität codiert als:

```text
spiffe://baseharbor/apps/<app>/<environment>
```

## Laufzeit-API

Canonische anwendungsgebundene Runtime-Routen leben unten `/runtime/v1`.

Verwaltete Geheimrouten:

```text
POST   /runtime/v1/secrets
POST   /runtime/v1/secrets/resolve
PUT    /runtime/v1/secrets/resolve
DELETE /runtime/v1/secrets/resolve
```

Laufzeitressource und asynchrone Betriebsrouten verwenden den gleichen Brokernamensraum. Bestehende app-qualifizierte Geheimrouten unter `/runtime/v1/apps/{app}/...` Aliasnamen für die Kompatibilität bleiben.

Die Laufzeitidentität authentifiziert sich nie an den normalen Operator API unter `/api/v1/...`.

Jede Identität ist auf genau eine Anwendung/Umgebung beschränkt. Eine Identität, die für eine Anwendung ausgegeben wird, kann nicht für den Broker einer anderen Anwendung wiederverwendet werden.

## API-Dokumentation für die Entwicklung

Wenn ein Broker benötigt wird in `dev` or ` development`, BaseHarbor stellt eingebettete Swagger/OpenAPI-Dokumentation auf einer separat zugewiesenen Host-Loopback-only-URL aus. Der Dokumentationshörer stellt keinen Authentifizierungs-Bypass in die mTLS-Laufzeit-API zur Verfügung.

Test/Staging und Produktion halten die interaktive Dokumentation standardmäßig deaktiviert.

## Rotation und Widerruf

Operatoren können Laufzeitidentitätsmaterial ungültig machen oder rotieren, ohne logische Ressource oder geheime Referenzen zu ändern.

Bestehende Geheimreferenzen wie:

```text
baseharbor://secrets/dyn-0123456789abcdef0123456789abcdef
```

über die Identitätsdrehung hinweg stabil bleiben.

Anwendungen, die keine BaseHarbor Runtime-Fähigkeit erfordern, erhalten keine Application Runtime Broker-Identität.
