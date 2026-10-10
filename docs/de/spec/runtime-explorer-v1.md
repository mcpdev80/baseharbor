# Runtime-Explorer-Vertrag v1

## Gültigkeitsbereich

Runtime Explorer ist der Anbieter-neutrale Inspektions- und begrenzter Low-Level-Betriebsvertrag für konkrete Laufzeitressourcen.

Es sitzt unter BaseHarbor Application Intention:

```text
Application / Deployment / Component-or-Provider
        -> runtime realization
        -> Runtime Explorer resource
```

Portable Application Intent MUSS NICHT Docker, Podman, Kubernetes oder OpenShift Ressourcenobjekte enthalten.

Vertragskennung:

```text
baseharbor.runtime-explorer/v1
```

## Stabile Ressourcenreferenz

Jede Ressource MUSS eine stabile Referenz mit:

- Identität des Laufzeitanbieters;
- Basis-Harbor-Ziel;
- Anbieterneutrale Ressourcenart;
- stabile Laufzeit-Ressource-ID.

Runtime-native Display-Namen sind Metadaten, nicht BaseHarbor logische Identität.

## Art der Ressourcen

v1 definiert additive Ressourcenarten für:

- `container`;
- `image`;
- `volume`;
- `network`;
- `pod`.

Zukünftige Kubernetes/OpenShift-Arten MÜSSEN hinzugefügt werden, ohne die portable Anwendung zu ändern.

## Eigentum

Die Kunden MÜSSEN die von Core/Runtime-Evidenz zurückgegebenen maßgeblichen Eigentümer verwenden.

```text
managed
external
unmanaged
platform
```

Mandanten MÜSSEN KEINE Besitztümer von Namen, Präfixen oder Etiketten allein ableiten.

A `managed` Ressource MUSS eine BaseHarbor-Beziehung zu einer Anwendung, Bereitstellung, Komponente oder Anbieter tragen.

## Beziehungen

Wo bekannt, deckt Runtime Explorer stabile Beziehungen auf:

```text
Application
  -> Deployment
    -> Component / Provider
      -> Runtime resource
```

Laufzeitspezifische Projekt-/Service-Etiketten MÜSSEN als Referenzen exponiert werden, MÜSSEN aber keine stabilen BaseHarbor-Identitäten ersetzen.

## Zustand und Gesundheit

Der gemeinsame Ressourcenzustand kann Folgendes umfassen:

- gewünschter Zustand, in dem er sinnvoll ist;
- beobachteter Zustand;
- Gesundheit;
- Bereitschaft;
- creation/update timestamps wenn die Laufzeit sie freilegt.

Anbieterspezifische Details gehören in das Erweiterungsfeld.

## Fähigkeiten

Clients MUSS Fähigkeiten verhandeln und MUSS NICHT Hardcode-Verhalten nach Provider-Namen.

Das Fähigkeitsvokabular umfasst:

```text
resources.inspect
resources.metrics
logs
container.lifecycle
container.exec
pod.inspect
```

Anbieter werben nur Fähigkeiten, die sie tatsächlich implementieren. Nicht unterstützte Fähigkeiten scheitern explizit.

## Vorhaben

Gebundene v1-Container-Betriebe sind:

- `start`;
- `stop`;
- `restart`;
- `exec`.

Die Maschinenschnittstelle entlarvt `runtime.start`, ` runtime.stop `und` runtime.restart`als separate autorisierte semantische Operationen. Dies hält die Sicherheit/Politik-Metadaten stabil und verhindert, dass ein generischer Operationsstring nach der Autorisierung seine Bedeutung ändert.

Exec benötigt einen expliziten Befehl und verwendet die geschützte Laufzeit-Exec-Stream-Grenze. Er impliziert keine Host-Shell und wird nicht als generischer Befehl übergeben.

Runtime Explorer MUSS keinen generischen Provider-Befehl durchpassen.

## Mutationssicherheit

Direkte Low-Level-Mutation unterscheidet sich von der bevorzugten semantischen BaseHarbor-Operation.

Managed resources entlarvt Abgleich Metadaten wie:

- bevorzugter semantischer Betrieb;
- ob direkte Mutation durch Versöhnung ersetzt werden kann;
- Menschenlesbares Detail.

Nicht verwaltete und externe Ressourcen MÜSSEN NICHT als BaseHarbor-eigene Mutation behandelt werden.

Alle maschinenseitigen Mutationen unterliegen weiterhin der gemeinsamen Maschinenbediener-Genehmigungs-, Richtlinien-, Sicherheits- und Audit-Grenze.

## Protokolle und Metriken

Der Protokollzugriff verwendet eine stabile Laufzeit-Ressource-Referenz und optional gebundene Historien-Selektoren.

Die geschützte Maschinen-HTTP-API aus dem Schwester-HTTP-Vertrag kann Runtime Explorer-Protokolle nach gemeinsamer Autorisierung streamen.

Metrics werden bei der Unterstützung über einen Provider-neutralen Handle dargestellt. Fehlende Laufzeitmetriken sind explizit und nicht synthetisiert.

Für einen zugelassenen Remote Connector werben Core `resources.metrics` nur wenn
die Live-Session annonciert `runtime.metrics`. Core bestätigt das ausgewählte Ziel,
Anbieter und beobachteten Container vor dem Versand. Eine erfolgreiche Antwort umfasst
eine fakultative `sample` mit `resource_id`, Kerneingangszeit ` observed_at`, und die
Einheimisch `cpu_percent`, ` memory_usage `und` network_io`Strings. Native Einheiten bleiben
Diese Beobachtungen bestätigen keine normalisierte Leistungsfähigkeit oder Gesundheit.
Fehlende native Sample-Returns `available: false`; Identitätsinkongruenzen und Misserfolge
verlangt Rückgabefehler.

## Docker und Podman Referenzrealisierung

Docker und Podman verwenden die bestehenden BaseHarbor Runtime Provider-Implementierungen.

Der Referenz-Explorer:

- Vorräte an Docker-Containern, -Bildern, -Volumen und -Netzwerken;
- Vorräte an Podman-Containern, Bildern, Volumen, Netzwerken und Pods;
- verwendet stabile Laufzeit-IDs, bei denen der Motor sie ausgibt; Volumennamen sind die Laufzeit-native stabile Kennungen;
- behält nicht verwaltete Ressourcen, anstatt sie fallen zu lassen;
- stellt Zustand und Gesundheit dar, in denen die Laufzeit sie liefert;
- Bild/Volumen/Netzwerk/Pod-Inventar wird nur in v1 gespeichert;
- Delegierte begrenzter Containerprotokolle/Lebenszyklus/exec durch ressourcenskopierte Laufzeitbefehle;
- fällt nie wieder auf eine Wirtsschale zurück.

Managed Container Ownership wird von BaseHarbor Deployment Evidence über einen separaten Resolver geliefert, anstatt aus Displaynamen zu erraten. Inventarressourcen ohne maßgebliche Besitznachweise bleiben erhalten `unmanaged`; Der Entdecker fördert sie nicht dazu, auf der Grundlage von Namenskonventionen verwaltet zu werden.

## Kubernetes/OpenShift-Kompatibilität

v1 ist absichtlich offen für additive Ressourcenarten, einschließlich:

- Namespaces;
- Arbeitsbelastungen;
- Hülsen,
- Dienstleistungen;
- Gateways/Einsteiger;
- ConfigMaps;
- geheime Metadaten;
- PVCs,
- Ereignisse;
- Knoten;
- OpenShift-spezifische Ressourcen.

Hinzufügen dieser Projektionen MÜSSEN KEINE Änderungen des portablen Anwendungsvertrags erfordern.
