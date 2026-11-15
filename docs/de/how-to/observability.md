# Requests beobachten

Verwende OpenTelemetry für Traces, normale Application-Logs für Logging und OpenMetrics für Metriken. Provider-Placement bleibt Deployment-Konfiguration.

Mit `baha` und konfiguriertem Runtime-Target:

```bash
baha app new observed-api --stack go --http --telemetry
cd observed-api
baha plan
baha up -e dev
baha doctor
```

Das Scaffold ergänzt OpenTelemetry SDK und OTLP-HTTP-Exporter. `.env.example` benennt Endpunkt, Service-Namen, Protokoll, Resource Attributes und Zertifikatsbindungen; geschützte Werte werden zur Laufzeit geliefert.

Nach Telemetrie-Initialisierung im eigenen HTTP-Handler:

```go
tracer := otel.Tracer("orders")
ctx, span := tracer.Start(request.Context(), "calculate-order-total")
defer span.End()
// Pass ctx to database and outbound HTTP operations.
```

`ctx` an Datenbank-/HTTP-Aufrufe weitergeben. Ein Exporter instrumentiert nicht automatisch jede Operation; ergänze Spans oder Instrumentierungsbibliotheken.

`baha app env --format json` zeigt maskierte Bindungen; `baha app logs app` liest Workload-Logs. Doctor verifiziert Capabilities, einen Fach-Span beweist erst ein tatsächlicher Request durch instrumentierten Code. Grafana oder andere Visualisierung ist optionales Ecosystem-Tooling. Für die Version v0.4.28 ist Perses die optionale, provider-basierte Visualisierungsoberfläche: sie wird bei v0.55.1 fixiert, ist keine Anwendungsfähigkeit und kein Teil des Pflicht-Core-Bundles, und wenn Perses nicht gewählt ist oder mit einer ungültigen Pin deklariert ist, ist das der Normalzust und kein Fehler — BaseHarbor meldet das als `not_configured` (fail-closed). Wenn Perses gewählt ist mit einer gültigen Pin, aber nicht bereitgestellt ist, meldet BaseHarbor die Visualisierungsoberfläche von Perses als `configured_unverified` statt „verified“: dieser Core definiert nur die Naht und liefert kein Perses-Health-Probe, daher bleibt Erreichbarkeit unverifiziert. Erstmals `verified` wird erst nach einer tatsächlichen Health-Probe von Perses zur Oberfläche erreicht.

[Kanonische Anleitung (EN)](https://mcpdev80.github.io/baseharbor/how-to/observability/).


Technische Bezeichner: `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`.
