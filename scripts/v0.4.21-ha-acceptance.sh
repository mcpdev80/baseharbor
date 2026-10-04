#!/usr/bin/env bash
set -euo pipefail

group="${1:-}"
runtime="${BASEHARBOR_TEST_RUNTIME:-}"

case "$runtime" in
  docker|podman) ;;
  *) echo "BASEHARBOR_TEST_RUNTIME must be docker or podman" >&2; exit 2 ;;
esac

reset_runtime() {
  bash scripts/ci-runtime-reset.sh "$runtime"
}

run_acceptance() {
  local label="$1"
  local env_name="$2"
  local env_value="$3"
  local package="$4"
  local test_name="$5"
  local timeout="$6"

  echo "::group::v0.4.21 HA · $group · $label · $runtime"
  reset_runtime
  env "$env_name=$env_value" \
    go test -p 1 "$package" -run "^${test_name}$" -count=1 -timeout "$timeout" -v
  reset_runtime
  echo "::endgroup::"
}

trap reset_runtime EXIT

case "$group" in
  data)
    run_acceptance "PostgreSQL + OpenBao control plane" \
      BASEHARBOR_RUNTIME_RESTART_ACCEPTANCE true \
      ./cmd/baha TestExistingControlPlaneRestartRequiresAndUsesRecoveryFile 20m
    run_acceptance "MongoDB" \
      BASEHARBOR_MONGODB_HA_ACCEPTANCE 1 \
      ./internal/application TestMongoDBHARuntimeFailoverAcceptanceInCI 12m
    run_acceptance "Valkey" \
      BASEHARBOR_VALKEY_HA_ACCEPTANCE 1 \
      ./internal/application TestValkeyHARuntimeFailoverAcceptanceInCI 12m
    run_acceptance "RabbitMQ" \
      BASEHARBOR_RABBITMQ_HA_ACCEPTANCE 1 \
      ./internal/application TestRabbitMQHARuntimeFailoverAcceptanceInCI 12m
    run_acceptance "SeaweedFS" \
      BASEHARBOR_SEAWEEDFS_HA_ACCEPTANCE 1 \
      ./internal/objectstorage TestSeaweedFSHARuntimeFailoverAcceptanceInCI 12m
    ;;
  valkey)
    run_acceptance "Valkey" \
      BASEHARBOR_VALKEY_HA_ACCEPTANCE 1 \
      ./internal/application TestValkeyHARuntimeFailoverAcceptanceInCI 12m
    ;;
  identity)
    run_acceptance "Keycloak" \
      BASEHARBOR_KEYCLOAK_HA_ACCEPTANCE 1 \
      ./internal/identityprovider TestKeycloakHARuntimeFailoverAcceptanceInCI 18m
    ;;
  observability)
    run_acceptance "OpenTelemetry Collector" \
      BASEHARBOR_OTEL_HA_ACCEPTANCE 1 \
      ./internal/telemetry TestOTelHARuntimeFailoverAcceptanceInCI 12m
    run_acceptance "Prometheus" \
      BASEHARBOR_PROMETHEUS_HA_ACCEPTANCE 1 \
      ./internal/metrics TestPrometheusHARuntimeFailoverAcceptanceInCI 12m
    run_acceptance "Loki" \
      BASEHARBOR_LOKI_HA_ACCEPTANCE 1 \
      ./internal/logs TestLokiHARuntimeFailoverAcceptanceInCI 12m
    run_acceptance "Tempo" \
      BASEHARBOR_TEMPO_HA_ACCEPTANCE 1 \
      ./internal/traces TestTempoHARuntimeFailoverAcceptanceInCI 18m
    ;;
  routing)
    run_acceptance "Gateway + management routing" \
      BASEHARBOR_GATEWAY_ACCEPTANCE 1 \
      ./internal/devgateway TestGatewayRuntimeContinuityAcceptanceInCI 10m
    ;;
  *)
    echo "Unknown v0.4.21 HA group: $group" >&2
    exit 2
    ;;
esac

echo "v0.4.21 HA group PASS: $group ($runtime)"
