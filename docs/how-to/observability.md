# Use observability

BaseHarbor treats metrics, logs and traces as capabilities and platform concerns rather than hard-coding one product into the application contract.

Current reference implementations may use products such as Prometheus, Loki, Alloy or Tempo, but applications should emit standard signals.

Use standard interfaces where applicable:

- OpenMetrics;
- OpenTelemetry / OTLP;
- normal application logs.

Run `baha doctor` to verify that configured observability capabilities are actually usable.

Visualization tooling such as Grafana is optional ecosystem tooling and is not part of the portable application contract.
