package logs

import (
	"fmt"
	"strconv"
	"strings"
)

func lokiConfig() string {
	return `auth_enabled: false
server:
  http_listen_address: 0.0.0.0
  http_listen_port: 3100
common:
  path_prefix: /loki
  replication_factor: 1
  ring:
    instance_addr: 127.0.0.1
    kvstore:
      store: inmemory
ingester:
  wal:
    enabled: true
    dir: /loki/wal
schema_config:
  configs:
    - from: 2020-05-15
      store: tsdb
      object_store: filesystem
      schema: v13
      index:
        prefix: index_
        period: 24h
storage_config:
  filesystem:
    directory: /loki/chunks
`
}

func alloyConfig(registrations []Registration) string {
	var b strings.Builder
	b.WriteString(`loki.relabel "syslog" {
  forward_to = [loki.write.local.receiver]

  rule {
    source_labels = ["__syslog_message_app_name"]
    target_label  = "baseharbor_service"
  }
}

loki.write "local" {
  endpoint {
    url = "http://loki:3100/loki/api/v1/push"
  }
}
`)
	for i, registration := range registrations {
		fmt.Fprintf(&b, "\nloki.source.syslog %s {\n", strconv.Quote(fmt.Sprintf("application_%d", i)))
		b.WriteString("  listener {\n")
		fmt.Fprintf(&b, "    address       = %s\n", strconv.Quote(fmt.Sprintf("0.0.0.0:%d", registration.SyslogPort)))
		b.WriteString("    protocol      = \"udp\"\n")
		b.WriteString("    syslog_format = \"rfc5424\"\n")
		fmt.Fprintf(&b, "    labels = { baseharbor_application = %s, baseharbor_environment = %s, baseharbor_source_class = \"application\" }\n", strconv.Quote(registration.Application), strconv.Quote(registration.Environment))
		b.WriteString("  }\n")
		b.WriteString("  relabel_rules = loki.relabel.syslog.rules\n")
		b.WriteString("  forward_to    = [loki.write.local.receiver]\n")
		b.WriteString("}\n")
	}
	return b.String()
}

func providerComposeYAML(placement Placement, registrations []Registration) string {
	var b strings.Builder
	b.WriteString("services:\n")
	b.WriteString("  loki:\n")
	fmt.Fprintf(&b, "    image: %s\n", LokiImage)
	fmt.Fprintf(&b, "    user: %s\n", strconv.Quote(fmt.Sprintf("%d:%d", LokiRuntimeUID, LokiRuntimeGID)))
	b.WriteString("    command: [\"-config.file=/etc/loki/loki.yaml\"]\n")
	b.WriteString("    read_only: true\n")
	b.WriteString("    cap_drop: [\"ALL\"]\n")
	b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
	b.WriteString("    tmpfs: [\"/tmp:rw,noexec,nosuid,nodev\"]\n")
	b.WriteString("    volumes:\n")
	b.WriteString("      - ./loki.yaml:/etc/loki/loki.yaml:ro\n")
	b.WriteString("      - loki-data:/loki\n")
	b.WriteString("    ports:\n")
	b.WriteString("      - \"127.0.0.1:${BASEHARBOR_LOKI_PORT}:3100\"\n")
	b.WriteString("    networks: [logs-internal, logs-publish]\n")
	b.WriteString("  alloy:\n")
	fmt.Fprintf(&b, "    image: %s\n", AlloyImage)
	fmt.Fprintf(&b, "    user: %s\n", strconv.Quote(fmt.Sprintf("%d:%d", AlloyRuntimeUID, AlloyRuntimeGID)))
	b.WriteString("    command: [\"run\", \"--server.http.listen-addr=127.0.0.1:12345\", \"--storage.path=/var/lib/alloy/data\", \"/etc/alloy/config.alloy\"]\n")
	b.WriteString("    read_only: true\n")
	b.WriteString("    cap_drop: [\"ALL\"]\n")
	b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
	b.WriteString("    tmpfs: [\"/tmp:rw,noexec,nosuid,nodev\"]\n")
	b.WriteString("    volumes:\n")
	b.WriteString("      - ./config.alloy:/etc/alloy/config.alloy:ro\n")
	b.WriteString("      - alloy-data:/var/lib/alloy/data\n")
	if len(registrations) > 0 {
		b.WriteString("    ports:\n")
		for _, registration := range registrations {
			fmt.Fprintf(&b, "      - %s\n", strconv.Quote(fmt.Sprintf("127.0.0.1:%d:%d/udp", registration.SyslogPort, registration.SyslogPort)))
		}
	}
	b.WriteString("    depends_on: [loki]\n")
	b.WriteString("    networks: [logs-internal, logs-publish]\n")
	b.WriteString("networks:\n")
	b.WriteString("  logs-internal:\n")
	b.WriteString("    internal: true\n")
	b.WriteString("  logs-publish:\n")
	b.WriteString("    driver: bridge\n")
	b.WriteString("volumes:\n")
	fmt.Fprintf(&b, "  loki-data:\n    name: %s\n", strconv.Quote(placement.LokiVolume))
	fmt.Fprintf(&b, "  alloy-data:\n    name: %s\n", strconv.Quote(placement.AlloyVolume))
	return b.String()
}
