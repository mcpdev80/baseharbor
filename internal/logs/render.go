package logs

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
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

func lokiHAConfig() string {
	return `auth_enabled: false
server:
  http_listen_address: 0.0.0.0
  http_listen_port: 3100
common:
  path_prefix: /loki
  replication_factor: 3
  compactor_grpc_address: loki-1:9095
  ring:
    kvstore:
      store: memberlist
memberlist:
  join_members:
    - loki-1:7946
    - loki-2:7946
    - loki-3:7946
  cluster_label: baseharbor-loki
  cluster_label_verification_disabled: false
ingester:
  lifecycler:
    unregister_on_shutdown: true
  wal:
    enabled: true
    dir: /loki/wal
    flush_on_shutdown: true
schema_config:
  configs:
    - from: 2024-04-01
      store: tsdb
      object_store: s3
      schema: v13
      index:
        prefix: index_
        period: 24h
storage_config:
  aws:
    endpoint: ${BASEHARBOR_LOKI_S3_ENDPOINT}
    region: us-east-1
    bucketnames: ${BASEHARBOR_LOKI_S3_BUCKET}
    access_key_id: ${BASEHARBOR_LOKI_S3_ACCESS_KEY_ID}
    secret_access_key: ${BASEHARBOR_LOKI_S3_SECRET_ACCESS_KEY}
    insecure: false
    s3forcepathstyle: true
    http_config:
      tls_ca_path: /run/baseharbor/object-storage/ca.pem
compactor:
  working_directory: /loki/compactor
`
}

func lokiHAAlloyConfig(config string, requireClient bool) string {
	const old = `    url = "http://loki:3100/loki/api/v1/push"
`
	var replacement strings.Builder
	replacement.WriteString("    url = \"https://loki:8443/loki/api/v1/push\"\n")
	replacement.WriteString("    tls_config {\n")
	replacement.WriteString("      ca_file     = \"/run/baseharbor/loki-access/ca.pem\"\n")
	if requireClient {
		replacement.WriteString("      cert_file   = \"/run/baseharbor/loki-access/client-cert.pem\"\n")
		replacement.WriteString("      key_file    = \"/run/baseharbor/loki-access/client-key.pem\"\n")
	}
	replacement.WriteString("      server_name = \"loki\"\n")
	replacement.WriteString("    }\n")
	return strings.Replace(config, old, replacement.String(), 1)
}

func alloyConfig(registrations []Registration) string {
	return alloyConfigForModeSources(registrations, nil, bhruntime.LogCollectionSyslog)
}

func alloyConfigForModeSources(registrations []Registration, providerSources []observability.SignalSource, mode bhruntime.LogCollectionMode, platformSyslogPort ...int) string {
	if mode == bhruntime.LogCollectionJournald {
		return alloyJournalConfig(registrations, providerSources)
	}
	port := 0
	if len(platformSyslogPort) > 0 {
		port = platformSyslogPort[0]
	}
	return alloySyslogConfig(registrations, providerSources, port)
}

func alloySyslogConfig(registrations []Registration, providerSources []observability.SignalSource, platformSyslogPort int) string {
	var b strings.Builder
	b.WriteString(`loki.relabel "syslog" {
  forward_to = [loki.write.local.receiver]

  rule {
    source_labels = ["__syslog_message_app_name"]
    target_label  = "baseharbor_service"
  }
}

loki.relabel "provider_syslog" {
  forward_to = [loki.write.local.receiver]

  rule {
    source_labels = ["__syslog_message_app_name"]
    regex         = "([^/]+)/(.+)"
    target_label  = "baseharbor_provider"
    replacement   = "$1"
  }

  rule {
    source_labels = ["__syslog_message_app_name"]
    regex         = "([^/]+)/(.+)"
    target_label  = "baseharbor_service"
    replacement   = "$2"
  }
}

loki.relabel "platform_provider_syslog" {
  forward_to = [loki.write.local.receiver]

  rule {
    source_labels = ["__syslog_message_app_name"]
    regex         = "([^/]+)/(.+)"
    target_label  = "baseharbor_provider"
    replacement   = "$1"
  }

  rule {
    source_labels = ["__syslog_message_app_name"]
    regex         = "([^/]+)/(.+)"
    target_label  = "baseharbor_service"
    replacement   = "$2"
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

		fmt.Fprintf(&b, "\nloki.source.syslog %s {\n", strconv.Quote(fmt.Sprintf("application_provider_%d", i)))
		b.WriteString("  listener {\n")
		fmt.Fprintf(&b, "    address       = %s\n", strconv.Quote(fmt.Sprintf("0.0.0.0:%d", registration.ProviderSyslogPort)))
		b.WriteString("    protocol      = \"udp\"\n")
		b.WriteString("    syslog_format = \"rfc5424\"\n")
		fmt.Fprintf(&b, "    labels = { baseharbor_application = %s, baseharbor_environment = %s, baseharbor_source_class = \"application-provider\" }\n", strconv.Quote(registration.Application), strconv.Quote(registration.Environment))
		b.WriteString("  }\n")
		b.WriteString("  relabel_rules = loki.relabel.provider_syslog.rules\n")
		b.WriteString("  forward_to    = [loki.write.local.receiver]\n")
		b.WriteString("}\n")
	}
	if platformSyslogPort > 0 && hasPlatformProviderLogs(providerSources) {
		b.WriteString("\nloki.source.syslog \"platform_provider\" {\n")
		b.WriteString("  listener {\n")
		fmt.Fprintf(&b, "    address       = %s\n", strconv.Quote(fmt.Sprintf("0.0.0.0:%d", platformSyslogPort)))
		b.WriteString("    protocol      = \"udp\"\n")
		b.WriteString("    syslog_format = \"rfc5424\"\n")
		b.WriteString("    labels = { baseharbor_source_class = \"platform-provider\" }\n")
		b.WriteString("  }\n")
		b.WriteString("  relabel_rules = loki.relabel.platform_provider_syslog.rules\n")
		b.WriteString("  forward_to    = [loki.write.local.receiver]\n")
		b.WriteString("}\n")
	}
	return b.String()
}

func alloyJournalConfig(registrations []Registration, providerSources []observability.SignalSource) string {
	var b strings.Builder
	b.WriteString(`loki.write "local" {
  endpoint {
    url = "http://loki:3100/loki/api/v1/push"
  }
}
`)
	for i, registration := range registrations {
		project := application.WorkloadProjectNameForNamespace(
			application.New(registration.Application, registration.Environment, false, false, false),
			registration.Namespace,
		)
		pattern := "^" + regexp.QuoteMeta(project) + "(?:_(.+)_[0-9]+|-(.+))$"
		label := fmt.Sprintf("application_%d", i)
		fmt.Fprintf(&b, "\nloki.relabel %s {\n", strconv.Quote(label))
		b.WriteString("  forward_to = []\n\n")
		b.WriteString("  rule {\n")
		b.WriteString("    source_labels = [\"__journal_container_name\"]\n")
		fmt.Fprintf(&b, "    regex         = %s\n", strconv.Quote(pattern))
		b.WriteString("    action        = \"keep\"\n")
		b.WriteString("  }\n\n")
		b.WriteString("  rule {\n")
		b.WriteString("    source_labels = [\"__journal_container_name\"]\n")
		fmt.Fprintf(&b, "    regex         = %s\n", strconv.Quote(pattern))
		b.WriteString("    target_label  = \"baseharbor_service\"\n")
		b.WriteString("    replacement   = \"$1$2\"\n")
		b.WriteString("  }\n\n")
		b.WriteString("  rule {\n")
		b.WriteString("    target_label = \"baseharbor_application\"\n")
		fmt.Fprintf(&b, "    replacement  = %s\n", strconv.Quote(registration.Application))
		b.WriteString("  }\n\n")
		b.WriteString("  rule {\n")
		b.WriteString("    target_label = \"baseharbor_environment\"\n")
		fmt.Fprintf(&b, "    replacement  = %s\n", strconv.Quote(registration.Environment))
		b.WriteString("  }\n")
		b.WriteString("}\n")

		fmt.Fprintf(&b, "\nloki.source.journal %s {\n", strconv.Quote(label))
		b.WriteString("  path          = \"/var/log/journal\"\n")
		b.WriteString("  max_age       = \"1h\"\n")
		fmt.Fprintf(&b, "  relabel_rules = loki.relabel.%s.rules\n", label)
		b.WriteString("  labels        = { baseharbor_source_class = \"application\" }\n")
		b.WriteString("  forward_to    = [loki.write.local.receiver]\n")
		b.WriteString("}\n")
	}

	for i, source := range providerSources {
		if source.Class != observability.SourceApplicationProvider && source.Class != observability.SourcePlatformProvider {
			continue
		}
		project, service, ok := observability.ParseRuntimeTarget(source.Target)
		if !ok {
			continue
		}
		var registration *Registration
		if source.Class == observability.SourceApplicationProvider {
			for j := range registrations {
				if registrations[j].Application == source.OwnerApplication {
					registration = &registrations[j]
					break
				}
			}
			if registration == nil {
				continue
			}
		}
		pattern := "^(?:" + regexp.QuoteMeta(project+"_"+service+"_") + "[0-9]+|" + regexp.QuoteMeta(project+"-"+service) + ")$"
		label := fmt.Sprintf("application_provider_%d", i)
		fmt.Fprintf(&b, "\nloki.relabel %s {\n", strconv.Quote(label))
		b.WriteString("  forward_to = []\n\n")
		b.WriteString("  rule {\n")
		b.WriteString("    source_labels = [\"__journal_container_name\"]\n")
		fmt.Fprintf(&b, "    regex         = %s\n", strconv.Quote(pattern))
		b.WriteString("    action        = \"keep\"\n")
		b.WriteString("  }\n\n")
		b.WriteString("  rule {\n")
		b.WriteString("    target_label = \"baseharbor_provider\"\n")
		fmt.Fprintf(&b, "    replacement  = %s\n", strconv.Quote(string(source.Provider)))
		b.WriteString("  }\n\n")
		b.WriteString("  rule {\n")
		b.WriteString("    target_label = \"baseharbor_service\"\n")
		fmt.Fprintf(&b, "    replacement  = %s\n", strconv.Quote(service))
		b.WriteString("  }\n\n")
		if registration != nil {
			b.WriteString("  rule {\n")
			b.WriteString("    target_label = \"baseharbor_application\"\n")
			fmt.Fprintf(&b, "    replacement  = %s\n", strconv.Quote(registration.Application))
			b.WriteString("  }\n\n")
			b.WriteString("  rule {\n")
			b.WriteString("    target_label = \"baseharbor_environment\"\n")
			fmt.Fprintf(&b, "    replacement  = %s\n", strconv.Quote(registration.Environment))
			b.WriteString("  }\n")
		}
		b.WriteString("}\n")

		fmt.Fprintf(&b, "\nloki.source.journal %s {\n", strconv.Quote(label))
		b.WriteString("  path          = \"/var/log/journal\"\n")
		b.WriteString("  max_age       = \"1h\"\n")
		fmt.Fprintf(&b, "  relabel_rules = loki.relabel.%s.rules\n", label)
		fmt.Fprintf(&b, "  labels        = { baseharbor_source_class = %s }\n", strconv.Quote(string(source.Class)))
		b.WriteString("  forward_to    = [loki.write.local.receiver]\n")
		b.WriteString("}\n")
	}
	return b.String()
}

func providerComposeYAML(placement Placement, registrations []Registration) string {
	return providerComposeYAMLForModeAndAccess(placement, registrations, bhruntime.LogCollectionSyslog, serviceaccess.HTTPGatewayFiles{
		Caddyfile: "./service-access/Caddyfile",
		Material:  serviceaccess.TLSMaterial{CA: "./service-access/runtime/ca.pem", ServerCertificate: "./service-access/runtime/server.pem", ServerKey: "./service-access/runtime/server-key.pem"},
	})
}

func providerComposeYAMLForModeAndAccess(placement Placement, registrations []Registration, mode bhruntime.LogCollectionMode, access serviceaccess.HTTPGatewayFiles, options ...any) string {
	platformPort := 0
	storageNetwork := ""
	for _, option := range options {
		switch value := option.(type) {
		case int:
			platformPort = value
		case string:
			storageNetwork = strings.TrimSpace(value)
		}
	}
	lokiService := "loki"
	alloyService := "alloy"
	accessSpec := lokiAccessSpec()
	if placement.Scope == capability.ScopeApplication {
		lokiService = "baseharbor-internal-loki"
		alloyService = "baseharbor-internal-alloy"
		accessSpec.ServiceName = "baseharbor-internal-loki-access"
	}
	var b strings.Builder
	b.WriteString("services:\n")
	if storageNetwork == "" {
		fmt.Fprintf(&b, "  %s:\n", lokiService)
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
		if placement.Scope == capability.ScopeApplication {
			b.WriteString("    networks:\n      logs-internal:\n        aliases:\n          - loki\n")
		} else {
			b.WriteString("    networks: [logs-internal]\n")
		}
	} else {
		for ordinal := 1; ordinal <= 3; ordinal++ {
			name := fmt.Sprintf("loki-%d", ordinal)
			fmt.Fprintf(&b, "  %s:\n", name)
			fmt.Fprintf(&b, "    image: %s\n", LokiImage)
			fmt.Fprintf(&b, "    user: %s\n", strconv.Quote(fmt.Sprintf("%d:%d", LokiRuntimeUID, LokiRuntimeGID)))
			mode := "worker"
			if ordinal == 1 {
				mode = "main"
			}
			fmt.Fprintf(&b, "    command: [\"-config.file=/etc/loki/loki.yaml\", \"-config.expand-env=true\", \"-target=all\", \"-compactor.horizontal-scaling-mode=%s\"]\n", mode)
			b.WriteString("    read_only: true\n")
			b.WriteString("    cap_drop: [\"ALL\"]\n")
			b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
			b.WriteString("    tmpfs: [\"/tmp:rw,noexec,nosuid,nodev\"]\n")
			b.WriteString("    environment:\n")
			b.WriteString("      BASEHARBOR_LOKI_S3_ENDPOINT: ${BASEHARBOR_LOKI_S3_ENDPOINT}\n")
			b.WriteString("      BASEHARBOR_LOKI_S3_BUCKET: ${BASEHARBOR_LOKI_S3_BUCKET}\n")
			b.WriteString("      BASEHARBOR_LOKI_S3_ACCESS_KEY_ID: ${BASEHARBOR_LOKI_S3_ACCESS_KEY_ID}\n")
			b.WriteString("      BASEHARBOR_LOKI_S3_SECRET_ACCESS_KEY: ${BASEHARBOR_LOKI_S3_SECRET_ACCESS_KEY}\n")
			b.WriteString("    volumes:\n")
			b.WriteString("      - ./loki.yaml:/etc/loki/loki.yaml:ro\n")
			b.WriteString("      - ./object-storage-ca.pem:/run/baseharbor/object-storage/ca.pem:ro\n")
			fmt.Fprintf(&b, "      - loki-data-%d:/loki\n", ordinal)
			b.WriteString("    networks:\n      logs-internal: {}\n      object-storage: {}\n")
		}
	}
	b.WriteString(serviceaccess.HTTPGatewayComposeService(access, accessSpec))
	fmt.Fprintf(&b, "  %s:\n", alloyService)
	fmt.Fprintf(&b, "    image: %s\n", AlloyImage)
	if mode == bhruntime.LogCollectionJournald {
		b.WriteString("    user: \"0:0\"\n")
		b.WriteString("    command: [\"run\", \"--server.http.listen-addr=127.0.0.1:12345\", \"--storage.path=/tmp/alloy-data\", \"/etc/alloy/config.alloy\"]\n")
	} else {
		fmt.Fprintf(&b, "    user: %s\n", strconv.Quote(fmt.Sprintf("%d:%d", AlloyRuntimeUID, AlloyRuntimeGID)))
		b.WriteString("    command: [\"run\", \"--server.http.listen-addr=127.0.0.1:12345\", \"--storage.path=/var/lib/alloy/data\", \"/etc/alloy/config.alloy\"]\n")
	}
	b.WriteString("    read_only: true\n")
	b.WriteString("    cap_drop: [\"ALL\"]\n")
	b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
	b.WriteString("    tmpfs: [\"/tmp:rw,noexec,nosuid,nodev\"]\n")
	b.WriteString("    volumes:\n")
	b.WriteString("      - ./config.alloy:/etc/alloy/config.alloy:ro\n")
	if storageNetwork != "" {
		b.WriteString("      - ./service-access/runtime:/run/baseharbor/loki-access:ro\n")
	}
	if mode == bhruntime.LogCollectionJournald {
		b.WriteString("      - /var/log/journal:/var/log/journal:ro\n")
		b.WriteString("      - /etc/machine-id:/etc/machine-id:ro\n")
	} else {
		b.WriteString("      - alloy-data:/var/lib/alloy/data\n")
	}
	if mode == bhruntime.LogCollectionJournald {
		// Journal collection does not need a host-published syslog listener.
	} else if len(registrations) > 0 {
		b.WriteString("    ports:\n")
		for _, registration := range registrations {
			fmt.Fprintf(&b, "      - %s\n", strconv.Quote(fmt.Sprintf("127.0.0.1:%d:%d/udp", registration.SyslogPort, registration.SyslogPort)))
			fmt.Fprintf(&b, "      - %s\n", strconv.Quote(fmt.Sprintf("127.0.0.1:%d:%d/udp", registration.ProviderSyslogPort, registration.ProviderSyslogPort)))
		}
		if platformPort > 0 {
			fmt.Fprintf(&b, "      - %s\n", strconv.Quote(fmt.Sprintf("127.0.0.1:%d:%d/udp", platformPort, platformPort)))
		}
	}
	if storageNetwork == "" {
		fmt.Fprintf(&b, "    depends_on: [%s]\n", lokiService)
	} else {
		b.WriteString("    depends_on: [loki-access]\n")
	}
	b.WriteString("    networks: [logs-internal, logs-publish]\n")
	b.WriteString("networks:\n")
	b.WriteString("  logs-internal:\n")
	b.WriteString("    internal: true\n")
	fmt.Fprintf(&b, "    name: %s\n", strconv.Quote(placement.Network))
	if storageNetwork != "" {
		b.WriteString("  object-storage:\n    external: true\n")
		fmt.Fprintf(&b, "    name: %s\n", strconv.Quote(storageNetwork))
	}
	b.WriteString("  logs-publish:\n")
	b.WriteString("    driver: bridge\n")
	b.WriteString("volumes:\n")
	if storageNetwork == "" {
		fmt.Fprintf(&b, "  loki-data:\n    name: %s\n", strconv.Quote(placement.LokiVolume))
	} else {
		for ordinal := 1; ordinal <= 3; ordinal++ {
			name := placement.LokiVolume
			if ordinal > 1 {
				name = fmt.Sprintf("%s-replica-%d", placement.LokiVolume, ordinal)
			}
			fmt.Fprintf(&b, "  loki-data-%d:\n    name: %s\n", ordinal, strconv.Quote(name))
		}
	}
	fmt.Fprintf(&b, "  alloy-data:\n    name: %s\n", strconv.Quote(placement.AlloyVolume))
	return b.String()
}
