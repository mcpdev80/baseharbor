# ADR 0010: Evolvierende Anwendungsabsicht und zerstörungsfreie Aussöhnung

## Status

Akzeptiert als Fundament der Architektur nach v0.4.7 und vor v0.4.8.

## Kontext

BaseHarbor muss Anwendungen unterstützen, die sich kontinuierlich weiterentwickeln. Ein Repository kann mit PostgreSQL beginnen, Redis später hinzufügen, dann Objektspeicher, Metriken, Telemetrie oder andere Funktionen. Repository-Inspektion kann daher kein einmaliger Bootstrap-Helfer sein, und der Anwendungsvertrag kann nicht als eingefrorenes Installationsrezept behandelt werden.

Die gleiche Anwendung kann auch zwei verschiedene Ressourcenmomente benötigen:

1. Bereitstellungszeitressourcen, die BaseHarbor vor Beginn der Arbeitsbelastungen vorschreibt und bindet;
2. Anwendungs-Zeit-Ressourcen, die ein bereits laufendes Workload später anfordern kann, zum Beispiel eine logische S3-Speicherressource pro Mieter.

Gleichzeitig muss der portable Vertrag klein bleiben. Reiche Repository-Evidenz und Provider-Details dürfen nicht blind in demittierte YAML kopiert werden.

## Entscheidung

### 1. Der Bewerbungsvertrag ist spärlich gewünschten Zustand

Anwendungen deklarieren nur Fähigkeiten, die sie benötigen und nur Werte, die sich von sicheren Standardeinstellungen unterscheiden. Obligierte optionale Funktionen bedeuten nicht angefordert.

Sichtbare v1 Leser bleiben rückseitig kompatibel mit bestehenden expliziten `enabled: false` Input. Input.

### 2. Inspektion ist kontinuierlich und Delta-orientiert

`baha app inspect` ist während der Entwicklung immer wieder sicher zu laufen.

Die Inspektion liefert Belege für das Endlager und versöhnt es mit dem ausdrücklichen Vertrag in vier Staaten:

- `satisfied`: deklarierte Absicht enthält Belege für das Endlager;
- `new`: starke Beweise für eine nicht angemeldete Fähigkeit;
- `ambiguous`: schwächere Beweise bestehen und erfordern ein Urteil des Entwicklers;
- `stale`: erklärte Absicht wurde in der aktuellen Inspektion nicht wiederentdeckt.

Ein abgestandenes Ergebnis ist nur informationshalber. Es bedeutet niemals Entfernung.

### 3. Expliziter Vertrag gewinnt über Inferenz

Die Repository-Inspektion ist schreibgeschützt und beratend. Sie überschreibt niemals ein vorhandenes Manifest, beseitigt niemals eine Fähigkeit, weil Beweise verschwunden sind, und gewährt niemals eine Laufzeitgenehmigung von Source-Code-Evidenz.

Ergänzungen können als minimale Deltas vorgeschlagen werden. Ruhestand ist immer eine explizite Entscheidung für Entwickler/Operator.

### 4. Capability-Richtung ist erstklassige Inspektion Semantik

Nachweise können beschreiben, dass ein Antrag:

- `consume` s eine Fähigkeit,
- `provide` s/exponiert eine Fähigkeit,
- `export` s Daten;
- `receive` s Daten;
- kann `provision` logische Ressourcen zur Laufzeit.

Die ersten konkreten Beispiele sind PostgreSQL/Redis/S3 Verbrauch, OpenMetrics über einen Anwendungsendpunkt und OTLP Export.

### 5. Runtime Operationen gehören zu der Fähigkeit, nicht das Produkt

Eine Fähigkeit kann später genehmigte Laufzeitoperationen aufdecken, wie z. B.:

- `runtime.create`;
- `runtime.get`;
- `runtime.delete`;
- `runtime.rotate`.

Zum Beispiel, Quellcode, der einen S3-kompatiblen Aufruf `CreateBucket` Der Betrieb kann als Nachweis dafür gemeldet werden, dass der Antrag möglicherweise erforderlich ist.`object-storage.s3 ` ` runtime.create`. Diese Beweise erlauben nicht selbst die Schaffung von Eimern.

Statische und Laufzeitressourcen bleiben unter der gleichen logischen Fähigkeit und dem gleichen Providervertrag. BaseHarbor darf keine parallele SeaweedFS/AWS/Ceph-spezifische Laufzeitarchitektur erstellen.

### 6. Runtime Resource API verwendet OpenAPI und umgebungsskopierte interaktive Docs

Die anwendungsorientierte Runtime Resource API verwendet HTTP/JSON mit einem versionierten OpenAPI 3.1-Vertrag. OpenAPI ist normativ; Swagger UI, Scalar, Redoc oder ein anderer Renderer ist nur Präsentation.

Die anwendungsorientierte Runtime Resource API wird durch den pro-application **Application Runtime Broker** gehostet, der auch gemanaged-secret Runtime-Routen führt.`https://baseharbor-runtime:8443 `, die vorhergehende` baseharbor-secrets`DNS-Alias bleibt für Kompatibilität.

Interaktive Dokumentation folgt der Einführungspolitik:

- Entwicklung: standardmäßig aktiviert;
- test/staging: standardmäßig deaktiviert und opt-in;
- Produktion: standardmäßig deaktiviert und Opt-in.

Die Entwicklungsdokumentation wird von einem separaten eingebetteten Swagger/OpenAPI-Hörer bereitgestellt und nur auf einem automatisch zugewiesenen Host-Loopback-Port veröffentlicht.

Diese Einstellung ist keine Anwendungsabsicht und darf nicht portabel werden `baseharbor.yaml` Feld. Das Aktivieren interaktiver Dokumentation ändert niemals die API-Authentifizierung/Autorisierung.

Mutierende Laufzeitoperationen erfordern Idempotenz, können asynchrone Betriebsidentitäten zurückgeben und die bestehende Grenze für die sichere Bindung/Laufzeit-Identität für Anmeldeinformationen verwenden.

### 7. Inspektion kann reicher sein als die begangene Manifest

Das Inspektionsmodell kann Evidenzpfade, Vertrauen, Fähigkeitsrichtung und Runtime-Operation Hinweise behalten. Der engagierte Bewerbungsvertrag bleibt bewusst kleiner und Provider-neutral.

Dadurch kann das System die Code-Evolution verstehen, ohne jedes Detail der erkannten Implementierung in YAML umzuwandeln.

### 8. Entwicklungs-Arbeitsbaum ist gültige deployable Quelle

Für eine Entwicklungsumgebung ist die nicht gebundene Repository-Quelle gültige Eingabe in die normale Konvergenz. Entwickler müssen keine kleine Änderung des Anwendungscodes vornehmen oder verschieben, um sie nur über die lokale BaseHarbor-Laufzeit laufen zu lassen.

`baha up` unterscheidet den gewünschten Fingerabdruck des gesamten Repositorys von einem schmaleren Fingerabdruck des Control-State, der den portablen Anwendungsvertrag und die geschützten Bereitstellungseinstellungen abdeckt. Wenn nur die Repository-Workload-Quelle/Konfiguration geändert wurde und die vorhandene Bereitstellung READY ist, kann BaseHarbor einen Pfad zur loadload-only-Abgleichung verwenden. Änderungen der Kontrakt- oder Depository-Kontrolle werden durch die vollständige Fähigkeit/Provider-Abgleichung fortgesetzt.

Dies schwächt die Sicherheitssemantik von Git nicht.`baha app update` ist die separate Operation, die Git-Quelle von einem stromaufwärts mutiert und daher sauberer Baum, schnell vorwärts-nur und nicht-zerstörerisch bleibt.

## Folgen

- BaseHarbor kann eine Anwendung vom ersten Bootstrap durch spätere Fähigkeitszusätze begleiten.
- Bestehende Manifest-V1-Dateien bleiben lesbar.
- Canonical manifest output wird spärlich und verzichtet auf deaktivierte Fähigkeiten.
- Zukunft `baha up` Versöhnung kann den gleichen Inspect/Reconcile-Kern wiederverwenden, anstatt einen zweiten Scanner zu erfinden.
- Zukünftige Runtime-Ressource-APIs müssen Operationen explizit autorisieren und über die bestehende Capability/Provider-Grenze auflösen.
- Metrics/Prometheus arbeiten in v0.4.8 kann anwendungsspezifische OpenMetrics getrennt vom ausgewählten Metriken-Backend/Provider modellieren.
- Fehlende Beweise können niemals zu zerstörerischen Infrastrukturveränderungen führen.
