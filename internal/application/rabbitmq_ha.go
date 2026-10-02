package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
