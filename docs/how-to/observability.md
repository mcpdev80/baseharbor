# Trace requests in a Go API

Use OpenTelemetry for application traces, standard application logs for logging, and OpenMetrics where metrics are exposed. Provider placement remains deployment configuration.

## Create a traced API scaffold

From a parent directory, with `baha` installed:

```bash
baha app new observed-api --stack go --http --telemetry
cd observed-api
baha plan
baha up -e dev
baha doctor
```

`up` requires a configured runtime Target. The scaffold adds the OpenTelemetry SDK and OTLP HTTP trace exporter. It prepares `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`, protocol, resource attributes and certificate bindings in `.env.example`.

## Trace a business operation

After the generated telemetry setup, add a span around an order calculation:

```go
tracer := otel.Tracer("orders")
ctx, span := tracer.Start(request.Context(), "calculate-order-total")
defer span.End()
// Pass ctx to database and outbound HTTP operations.
```

Here `request` is the HTTP handler's request. A configured exporter does not automatically instrument every operation: add spans or instrumentation libraries to your application.

Check the effective bindings with `baha app env --format json` and inspect workload logs with `baha app logs app`. `baha doctor` verifies configured capabilities; seeing your business span in a collector or trace UI requires sending an actual request through the instrumented code. Grafana or another visualization tool is optional ecosystem tooling.
