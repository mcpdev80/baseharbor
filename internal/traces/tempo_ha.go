package traces

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const redpandaImage = "docker.redpanda.com/redpandadata/redpanda:v26.2.3"

func tempoHAConfig() string {
	return `server:
  http_listen_address: 0.0.0.0
  http_listen_port: 3200
  grpc_listen_address: 0.0.0.0
  grpc_listen_port: 9095

distributor:
  receivers:
    otlp:
      protocols:
        http:
          endpoint: 0.0.0.0:4318

ingest:
  kafka:
    address: redpanda-0:9092,redpanda-1:9092,redpanda-2:9092
    topic: tempo-traces
    client_id: baseharbor-tempo
    auto_create_topic_enabled: false

block_builder:
  partitions_per_instance: 1
  consume_cycle_duration: 30s
  wal:
    path: /var/tempo/block-builder/traces

live_store:
  ring:
    kvstore:
      store: memberlist
  partition_ring:
    kvstore:
      store: memberlist
    min_partition_owners_count: 2
    min_partition_owners_duration: 5s
  wal:
    path: /var/tempo/live-store/traces
  readiness_target_lag: 2s
  fail_on_high_lag: false
  remove_owner_on_shutdown: true

querier:
  frontend_worker:
    frontend_address: tempo-query-frontend:9095
  partition_ring:
    minimize_requests: true
    hedging_delay: 1s

backend_worker:
  backend_scheduler_addr: tempo-backend-scheduler:9095
  ring:
    kvstore:
      store: memberlist

memberlist:
  join_members:
    - tempo-distributor-1:7946
    - tempo-distributor-2:7946
    - tempo-live-a-0:7946
    - tempo-live-a-1:7946
    - tempo-live-a-2:7946
    - tempo-live-b-0:7946
    - tempo-live-b-1:7946
    - tempo-live-b-2:7946
    - tempo-querier-1:7946
    - tempo-querier-2:7946
    - tempo-backend-worker-1:7946
    - tempo-backend-worker-2:7946

storage:
  trace:
    backend: s3
    s3:
      endpoint: ${BASEHARBOR_TEMPO_S3_ENDPOINT}
      bucket: ${BASEHARBOR_TEMPO_S3_BUCKET}
      access_key: ${BASEHARBOR_TEMPO_S3_ACCESS_KEY_ID}
      secret_key: ${BASEHARBOR_TEMPO_S3_SECRET_ACCESS_KEY}
      insecure: false
      tls_ca_path: /run/baseharbor/object-storage/ca.pem
      tls_server_name: seaweedfs
      tls_insecure_skip_verify: false
    wal:
      path: /var/tempo/wal

usage_report:
  reporting_enabled: false
`
}

func tempoHACompose(p Placement, access serviceaccess.HTTPGatewayFiles, storage objectstorage.PlatformBucket) string {
	var b strings.Builder
	b.WriteString("services:\n")
	renderRedpanda := func(ordinal int) {
		name := fmt.Sprintf("redpanda-%d", ordinal)
		fmt.Fprintf(&b, "  %s:\n", name)
		fmt.Fprintf(&b, "    image: %s\n", redpandaImage)
		b.WriteString("    restart: unless-stopped\n")
		b.WriteString("    command:\n")
		b.WriteString("      - redpanda\n      - start\n")
		b.WriteString("      - --kafka-addr=internal://0.0.0.0:9092\n")
		fmt.Fprintf(&b, "      - --advertise-kafka-addr=internal://%s:9092\n", name)
		fmt.Fprintf(&b, "      - --rpc-addr=%s:33145\n", name)
		fmt.Fprintf(&b, "      - --advertise-rpc-addr=%s:33145\n", name)
		b.WriteString("      - --mode=dev-container\n      - --smp=1\n      - --default-log-level=info\n")
		if ordinal > 0 {
			b.WriteString("      - --seeds=redpanda-0:33145\n")
		}
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - redpanda-%d:/var/lib/redpanda/data\n", ordinal)
		b.WriteString("    networks: [traces]\n")
	}
	for ordinal := 0; ordinal < 3; ordinal++ {
		renderRedpanda(ordinal)
	}
	b.WriteString(`  tempo-kafka-init:
    image: ` + redpandaImage + `
    restart: "no"
    depends_on: [redpanda-0, redpanda-1, redpanda-2]
    entrypoint: ["/bin/bash", "-ec"]
    command:
      - |
        brokers=redpanda-0:9092,redpanda-1:9092,redpanda-2:9092
        attempts=0
        until rpk cluster health -X brokers="$$brokers" >/dev/null 2>&1; do
          attempts=$$((attempts+1))
          if [ "$$attempts" -ge 45 ]; then
            echo "Tempo Redpanda cluster did not become healthy" >&2
            rpk cluster health -X brokers="$$brokers" || true
            exit 1
          fi
          sleep 1
        done
        rpk topic create tempo-traces -X brokers="$$brokers" --partitions 3 --replicas 3 --topic-config min.insync.replicas=2 || true
    networks: [traces]

`)

	commonVolumes := func(wal string) string {
		var v strings.Builder
		v.WriteString("      - ./tempo.yaml:/etc/tempo/tempo.yaml:ro\n")
		v.WriteString("      - ./object-storage-ca.pem:/run/baseharbor/object-storage/ca.pem:ro\n")
		if wal != "" {
			fmt.Fprintf(&v, "      - %s:/var/tempo\n", wal)
		}
		return v.String()
	}
	commonEnv := `    environment:
      BASEHARBOR_TEMPO_S3_ENDPOINT: ${BASEHARBOR_TEMPO_S3_ENDPOINT}
      BASEHARBOR_TEMPO_S3_BUCKET: ${BASEHARBOR_TEMPO_S3_BUCKET}
      BASEHARBOR_TEMPO_S3_ACCESS_KEY_ID: ${BASEHARBOR_TEMPO_S3_ACCESS_KEY_ID}
      BASEHARBOR_TEMPO_S3_SECRET_ACCESS_KEY: ${BASEHARBOR_TEMPO_S3_SECRET_ACCESS_KEY}
`
	renderTempo := func(name, target string, extraArgs []string, wal string, aliases []string) {
		fmt.Fprintf(&b, "  %s:\n", name)
		fmt.Fprintf(&b, "    image: %s\n", ProviderImage)
		b.WriteString("    restart: unless-stopped\n")
		b.WriteString("    user: \"10001:10001\"\n")
		b.WriteString("    read_only: true\n")
		b.WriteString("    cap_drop: [\"ALL\"]\n")
		b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
		b.WriteString("    tmpfs: [\"/tmp:rw,noexec,nosuid,nodev\"]\n")
		b.WriteString("    command:\n")
		b.WriteString("      - -config.file=/etc/tempo/tempo.yaml\n")
		b.WriteString("      - -config.expand-env=true\n")
		fmt.Fprintf(&b, "      - -target=%s\n", target)
		for _, arg := range extraArgs {
			fmt.Fprintf(&b, "      - %s\n", arg)
		}
		b.WriteString(commonEnv)
		b.WriteString("    volumes:\n")
		b.WriteString(commonVolumes(wal))
		b.WriteString("    depends_on:\n      tempo-kafka-init:\n        condition: service_completed_successfully\n")
		b.WriteString("    networks:\n      traces:")
		if len(aliases) == 0 {
			b.WriteString(" {}\n")
		} else {
			b.WriteString("\n        aliases:\n")
			for _, alias := range aliases {
				fmt.Fprintf(&b, "          - %s\n", alias)
			}
		}
		b.WriteString("      object-storage: {}\n")
	}

	renderTempo("tempo-distributor-1", "distributor", nil, "", []string{"tempo"})
	renderTempo("tempo-distributor-2", "distributor", nil, "", []string{"tempo"})

	for _, zone := range []string{"a", "b"} {
		for partition := 0; partition < 3; partition++ {
			name := fmt.Sprintf("tempo-live-%s-%d", zone, partition)
			args := []string{
				"-live-store.ring.instance-id=" + name,
				"-live-store.ring.instance-zone=zone-" + zone,
			}
			renderTempo(name, "live-store", args, fmt.Sprintf("tempo-live-%s-%d", zone, partition), nil)
		}
	}
	for partition := 0; partition < 3; partition++ {
		name := fmt.Sprintf("tempo-block-builder-%d", partition)
		args := []string{"-block-builder.instance-id=" + name}
		renderTempo(name, "block-builder", args, fmt.Sprintf("tempo-block-builder-%d", partition), nil)
	}

	renderTempo("tempo-query-frontend-1", "query-frontend", nil, "", []string{"tempo-query-frontend"})
	renderTempo("tempo-query-frontend-2", "query-frontend", nil, "", []string{"tempo-query-frontend"})
	renderTempo("tempo-querier-1", "querier", nil, "", nil)
	renderTempo("tempo-querier-2", "querier", nil, "", nil)
	renderTempo("tempo-backend-scheduler", "backend-scheduler", nil, "tempo-backend-scheduler", []string{"tempo-backend-scheduler"})
	renderTempo("tempo-backend-worker-1", "backend-worker", nil, "tempo-backend-worker-1", nil)
	renderTempo("tempo-backend-worker-2", "backend-worker", nil, "tempo-backend-worker-2", nil)

	accessSpec := tempoHAQueryAccessSpec()
	b.WriteString(serviceaccess.HTTPGatewayComposeService(access, accessSpec))

	b.WriteString("networks:\n")
	fmt.Fprintf(&b, "  traces:\n    name: %s\n", strconv.Quote(p.Network))
	fmt.Fprintf(&b, "  object-storage:\n    external: true\n    name: %s\n", strconv.Quote(storage.Network))
	b.WriteString("volumes:\n")
	for ordinal := 0; ordinal < 3; ordinal++ {
		fmt.Fprintf(&b, "  redpanda-%d:\n", ordinal)
	}
	for _, zone := range []string{"a", "b"} {
		for partition := 0; partition < 3; partition++ {
			fmt.Fprintf(&b, "  tempo-live-%s-%d:\n", zone, partition)
		}
	}
	for partition := 0; partition < 3; partition++ {
		fmt.Fprintf(&b, "  tempo-block-builder-%d:\n", partition)
	}
	b.WriteString("  tempo-backend-scheduler:\n  tempo-backend-worker-1:\n  tempo-backend-worker-2:\n")
	return b.String()
}

func tempoHAQueryAccessSpec() serviceaccess.HTTPGatewaySpec {
	return serviceaccess.HTTPGatewaySpec{
		ServiceName:      "tempo-access",
		Upstreams:        []string{"http://tempo-query-frontend-1:3200", "http://tempo-query-frontend-2:3200"},
		PublishedPortEnv: "BASEHARBOR_TEMPO_PORT",
		ContainerPort:    8443,
		Networks:         []string{"traces"},
		NetworkAliases:   []string{"tempo-api"},
		CertificateNames: []string{"tempo-api"},
		RequireClient:    true,
		HealthURI:        "/ready",
	}
}
