# Sicherheit der Laufzeit-Broker-Eigenschaften

Der Application Runtime Broker ist bewusst um die Blast-Radius-Reduktion und nicht um einen gemeinsamen Multi-Tenant-Service ausgelegt.

Ein Kompromiss eines Antragsmaklers darf keinen Zugang zu folgenden Bereichen gewähren:

- das Backend-Netzwerk einer anderen Anwendung;
- die Laufzeitidentität einer anderen Anwendung;
- OpenBao AppRolle oder geheimer Namespace einer anderen Anwendung;
- die AppRolle des BaseHarbor-Managers;
- die Docker/Podman-Buchse;
- das Host-Dateisystem außerhalb explizit eingehängter Laufzeit-Identitätsdateien.

Der Broker läuft daher pro Anwendung, nur auf zwei Netzwerken: dem Applikations-Backend-Netzwerk und dem internen `baseharbor-secrets` Netzwerk. Es hat keinen Host-Port für normalen Laufzeitverkehr veröffentlicht.

Die Authentifizierung ist die Verteidigung in der Tiefe: mTLS mit einem anwendungsspezifischen URI SAN wird an der TLS-Schicht benötigt, dann wird das anwendungsskopierte Laufzeit-Token verifiziert, dann begrenzt OpenBao den Broker unabhängig durch die AppRolle-Richtlinie der Anwendung.

## v0.4.5 sicheres Mapping

Die Implementierung des Brokers bleibt unverändert, aber seine stabile Sicherheitssemantik wird nun durch `secure-binding/v1`: SPIFFE-Workload-Identität, undurchsichtige Runtime-Credential/Trust-Referenzen, Metadaten mit der geringsten Priorität und Unterstützung für Rotation/Revokation.

Anbieterspezifische OpenBao/AppRolle-Details und konkrete Zertifikats-/Schlüsselpfade sind absichtlich nicht Teil dieses verbindlichen Modells.


## Zuhörer für die Entwicklungsdokumentation

Die interaktive Runtime API-Dokumentation ist ein separater Hörer von der authentifizierten mTLS Runtime API.

In `dev `/` development`, BaseHarbor darf diesen Dokumentationshörer nur für eine automatisch zugewiesene Host-Loopback-Adresse veröffentlichen:

```text
127.0.0.1:<allocated-port>
```

Der Hörer dient eingebetteten Swagger-UI-Assets und nur dem kanonischen OpenAPI-Dokument. Er stellt keine Runtime-API-Aufrufe vor, stellt keine Laufzeitträger-Token, Client-Privatschlüssel, OpenBao-Anmeldeinformationen oder sichere Bindungen aus und schwächt mTLS oder die Autorisierung auf der realen Runtime-API nicht.

Test/Staging und Produktion halten diesen Hörer deaktiviert, es sei denn, ein Bediener entscheidet sich ausdrücklich.
