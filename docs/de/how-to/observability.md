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
```

`ctx` an Datenbank-/HTTP-Aufrufe weitergeben. Ein Exporter instrumentiert nicht automatisch jede Operation; ergänze Spans oder Instrumentierungsbibliotheken.

`baha app env --format json` zeigt maskierte Bindungen; `baha app logs app` liest Workload-Logs. Doctor verifiziert Capabilities, einen Fach-Span beweist erst ein tatsächlicher Request durch instrumentierten Code. Grafana oder andere Visualisierung ist optionales Ecosystem-Tooling.

[Kanonische Anleitung (EN)](https://mcpdev80.github.io/baseharbor/how-to/observability/).
