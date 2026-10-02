package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func rabbitmqMemberCount(m Manifest) int {
	return managedHAMemberCount(m, "messaging", 3)
}

func rabbitmqMemberServiceName(instance string, ordinal int) string {
	base := runtimeServiceName("rabbitmq", instance)
	if ordinal <= 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, ordinal+1)
}

func rabbitmqMemberVolumeName(instance string, ordinal int) string {
	return rabbitmqMemberServiceName(instance, ordinal) + "-data"
}

func rabbitmqGatewayUpstreams(m Manifest, instance string) []serviceaccess.TCPGatewayUpstream {
	count := rabbitmqMemberCount(m)
	out := make([]serviceaccess.TCPGatewayUpstream, 0, count)
	for i := 0; i < count; i++ {
		service := rabbitmqMemberServiceName(instance, i)
		out = append(out, serviceaccess.TCPGatewayUpstream{
			Name: service,
			Host: service,
			Port: 5672,
		})
	}
	return out
}

func ensureRabbitMQHAConfigFiles(files RuntimeFiles, m Manifest) error {
	if rabbitmqMemberCount(m) <= 1 {
		return nil
	}
	for _, instance := range RabbitMQInstanceNames(m) {
		root := filepath.Join(files.Dir, "providers", "rabbitmq", instance, "runtime")
		if err := os.MkdirAll(root, 0o700); err != nil {
			return fmt.Errorf("create RabbitMQ HA runtime directory for %s: %w", instance, err)
		}
		var b strings.Builder
		b.WriteString("cluster_formation.peer_discovery_backend = classic_config\n")
		for i := 0; i < rabbitmqMemberCount(m); i++ {
			fmt.Fprintf(&b, "cluster_formation.classic_config.nodes.%d = rabbit@%s\n", i+1, rabbitmqMemberServiceName(instance, i))
		}
		b.WriteString("default_queue_type = quorum\n")
		fmt.Fprintf(&b, "quorum_queue.initial_cluster_size = %d\n", rabbitmqMemberCount(m))
		b.WriteString("cluster_partition_handling = pause_minority\n")
		if err := os.WriteFile(filepath.Join(root, "rabbitmq.conf"), []byte(b.String()), 0o644); err != nil {
			return fmt.Errorf("write RabbitMQ HA config for %s: %w", instance, err)
		}
	}
	return nil
}

func rabbitmqHAConfigMount(instance string) string {
	return "./" + filepath.ToSlash(filepath.Join("providers", "rabbitmq", instance, "runtime", "rabbitmq.conf")) +
		":/etc/rabbitmq/rabbitmq.conf:ro"
}

func writeRabbitMQComposeService(b *strings.Builder, m Manifest, instance string) {
	bootstrapUserKey := rabbitmqRuntimeKey(instance, "BOOTSTRAP_USER")
	bootstrapPasswordKey := rabbitmqRuntimeKey(instance, "BOOTSTRAP_PASSWORD")
	cookieKey := rabbitmqRuntimeKey(instance, "ERLANG_COOKIE")
	image := "docker.io/library/rabbitmq:4.3.6-alpine"
	if m.Services.MessagingManagementUI {
		image = "docker.io/library/rabbitmq:4.3.6-management-alpine"
	}
	for ordinal := 0; ordinal < rabbitmqMemberCount(m); ordinal++ {
		service := rabbitmqMemberServiceName(instance, ordinal)
		fmt.Fprintf(b, "  %s:\n", service)
		fmt.Fprintf(b, "    image: %s\n", image)
		b.WriteString("    restart: unless-stopped\n")
		fmt.Fprintf(b, "    hostname: %s\n", service)
		b.WriteString("    user: \"rabbitmq\"\n")
		b.WriteString("    read_only: true\n")
		b.WriteString("    cap_drop: [\"ALL\"]\n")
		b.WriteString("    cap_add: [\"CHOWN\", \"SETGID\", \"SETUID\"]\n")
		b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
		b.WriteString("    tmpfs:\n      - /tmp:rw,noexec,nosuid,nodev\n")
		b.WriteString("    environment:\n")
		fmt.Fprintf(b, "      RABBITMQ_DEFAULT_USER: ${%s}\n", bootstrapUserKey)
		fmt.Fprintf(b, "      RABBITMQ_DEFAULT_PASS: ${%s}\n", bootstrapPasswordKey)
		if rabbitmqMemberCount(m) > 1 {
			fmt.Fprintf(b, "      RABBITMQ_ERLANG_COOKIE: ${%s}\n", cookieKey)
			fmt.Fprintf(b, "      RABBITMQ_NODENAME: rabbit@%s\n", service)
		}
		b.WriteString("    volumes:\n")
		fmt.Fprintf(b, "      - %s:/var/lib/rabbitmq\n", rabbitmqMemberVolumeName(instance, ordinal))
		if rabbitmqMemberCount(m) > 1 {
			fmt.Fprintf(b, "      - %s\n", rabbitmqHAConfigMount(instance))
		}
		b.WriteString("    healthcheck:\n")
		b.WriteString("      test: [\"CMD-SHELL\", \"rabbitmq-diagnostics -q ping\"]\n")
		b.WriteString("      interval: 5s\n      timeout: 5s\n      retries: 12\n      start_period: 10s\n\n")
	}
}

func writeRabbitMQUIComposeService(b *strings.Builder, m Manifest, instance string) {
	service := rabbitmqUIServiceName(instance)
	portKey := rabbitmqUIHostPortKey(instance)
	routeName := rabbitmqUIRouteName(instance)
	fmt.Fprintf(b, `  %s:
    image: %s
    restart: unless-stopped
    user: "65532:65532"
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    entrypoint: ["/bin/sh", "-ec"]
    command:
      - cat /usr/bin/caddy > /run/baseharbor/caddy && chmod 0755 /run/baseharbor/caddy && exec /run/baseharbor/caddy run --config /etc/caddy/Caddyfile --adapter caddyfile
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
      - /run/baseharbor:rw,exec,nosuid,nodev,mode=1777
      - /data:rw,noexec,nosuid,nodev,mode=1777
      - /config:rw,noexec,nosuid,nodev,mode=1777
    ports:
      - "127.0.0.1:${%s}:8443"
    volumes:
      - ./providers/management-ui/rabbitmq/%s/Caddyfile:/etc/caddy/Caddyfile:ro
      - ./providers/management-ui/rabbitmq/%s/server.pem:/certs/server.pem:ro
      - ./providers/management-ui/rabbitmq/%s/server-key.pem:/certs/server-key.pem:ro
    networks:
      default:
        aliases:
          - %q

`, service, UIProxyImage, portKey, instance, instance, instance, devaccess.ApplicationAlias(m.Name, routeName))
}
