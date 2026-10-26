# Verwendungsverbrauch v1

## Anwendungsbereich

Der Anwendungsverbrauch definiert eine logische Application/Component-Schnittstelle, die eine andere logische Application/Component-Schnittstelle verbraucht.

Es unterscheidet sich von Capability-Provider-Bindungen, Runtime Networking, Service Meshs, Gateways und DNS-Implementierungen.

## Portable Herstelleridentität

Verbraucherhinweise:

```text
stable application_id
component
interface
optional logical protocol
```

Die derzeitige Anwendung `application_id` kann bei gleichem Anwendungsverbrauch weggelassen werden.

Portable Consumption Intent MUSS KEINE Runtime-native Adressen, Container/Pod Namen, Namespaces, Nodes, Repliken oder Provider-Produktidentität enthalten.

## Entschließung

Runtime/Provider Realisation löst die logische Herstellerreferenz auf einen stabilen Endpunkt oder eine Bindung.

Der Endpunkt ist Deployment State, keine portable Absicht.

Der gleiche Vertrag wird für die gleiche Anwendung und den cross-application Verbrauch verwendet. Workspace/Repository Mitgliedschaft und Laufzeitumfang ändern nicht die logische Referenz.

Ein Hersteller kann derzeit null, eine oder viele Laufzeiten haben. Skalierung, Umschuldung, Rollenersatz und Failover verändern nicht die konsumgebundene Identität.

Ungelöste Hersteller bleiben ungelöste Ergebnisse, anstatt stillschweigend in eine Runtime-native Identität umgeschrieben zu werden.
