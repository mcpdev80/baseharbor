package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestRabbitMQHARendersThreeMembersStableGatewayAndQuorumConfig(t *testing.T) {
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "events",
		Environment:   "prod",
		HA:            true,
		Services: Services{
			MessagingQueue:        true,
			MessagingManagementUI: true,
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}

	env, err := newRuntimeEnv(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"RABBITMQ_USER=baseharbor",
		"RABBITMQ_PASSWORD=",
		"RABBITMQ_ERLANG_COOKIE=",
	} {
		if !strings.Contains(env, want) {
			t.Fatalf("RabbitMQ HA env missing %q:\n%s", want, env)
		}
	}

	compose, err := RuntimeComposeYAMLForProject(m, "bh-events")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"rabbitmq:",
		"rabbitmq-2:",
		"rabbitmq-3:",
		"RABBITMQ_NODENAME: rabbit@rabbitmq",
		"RABBITMQ_NODENAME: rabbit@rabbitmq-2",
		"RABBITMQ_NODENAME: rabbit@rabbitmq-3",
		"RABBITMQ_ERLANG_COOKIE: ${RABBITMQ_ERLANG_COOKIE}",
		"rabbitmq-data:/var/lib/rabbitmq",
		"rabbitmq-2-data:/var/lib/rabbitmq",
		"rabbitmq-3-data:/var/lib/rabbitmq",
		"name: bh-events_rabbitmq-data",
		"name: bh-events_rabbitmq-2-data",
		"name: bh-events_rabbitmq-3-data",
	} {
		if !strings.Contains(compose, want) {
			t.Fatalf("RabbitMQ HA compose missing %q:\n%s", want, compose)
		}
	}

	upstreams := rabbitmqGatewayUpstreams(m, defaultServiceInstance)
	if len(upstreams) != 3 {
		t.Fatalf("RabbitMQ HA gateway upstream count = %d, want 3", len(upstreams))
	}
	for i, want := range []string{"rabbitmq", "rabbitmq-2", "rabbitmq-3"} {
		if upstreams[i].Host != want || upstreams[i].Port != 5672 {
			t.Fatalf("RabbitMQ HA gateway upstream %d = %#v, want host %q port 5672", i, upstreams[i], want)
		}
	}

	files := RuntimeFiles{Dir: t.TempDir()}
	if err := ensureRabbitMQHAConfigFiles(files, m); err != nil {
		t.Fatal(err)
	}
	cfg, err := os.ReadFile(filepath.Join(files.Dir, "providers", "rabbitmq", defaultServiceInstance, "runtime", "rabbitmq.conf"))
	if err != nil {
		t.Fatal(err)
	}
	config := string(cfg)
	for _, want := range []string{
		"cluster_formation.peer_discovery_backend = classic_config",
		"cluster_formation.classic_config.nodes.1 = rabbit@rabbitmq",
		"cluster_formation.classic_config.nodes.2 = rabbit@rabbitmq-2",
		"cluster_formation.classic_config.nodes.3 = rabbit@rabbitmq-3",
		"default_queue_type = quorum",
		"quorum_queue.initial_cluster_size = 3",
		"cluster_partition_handling = pause_minority",
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("RabbitMQ HA config missing %q:\n%s", want, config)
		}
	}
}

func TestRabbitMQHAManagementUIRoutesAcrossAllMembers(t *testing.T) {
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "events",
		Environment:   "dev",
		HA:            true,
		Services: Services{
			MessagingQueue:        true,
			MessagingManagementUI: true,
		},
	}
	files := RuntimeFiles{Dir: t.TempDir()}
	if err := ensureRabbitMQManagementUI(context.Background(), serviceissuer.New(t), files, m); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(files.Dir, "providers", "management-ui", "rabbitmq", defaultServiceInstance, "Caddyfile"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "reverse_proxy rabbitmq:15672 rabbitmq-2:15672 rabbitmq-3:15672") {
		t.Fatalf("RabbitMQ HA management proxy is not member-aware:\n%s", got)
	}
}

func TestRabbitMQHAExplicitCardinalityIsRespected(t *testing.T) {
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "events",
		Environment:   "prod",
		HA:            true,
		Services:      Services{MessagingQueue: true},
	}
	m = WithAvailabilityOverride(m, "messaging", nil, 5)
	if got := rabbitmqMemberCount(m); got != 5 {
		t.Fatalf("RabbitMQ HA member count = %d, want 5", got)
	}
	upstreams := rabbitmqGatewayUpstreams(m, defaultServiceInstance)
	if got := len(upstreams); got != 5 {
		t.Fatalf("RabbitMQ HA gateway upstream count = %d, want 5", got)
	}
	if upstreams[0].Host != "rabbitmq" || upstreams[4].Host != "rabbitmq-5" {
		t.Fatalf("RabbitMQ HA gateway upstreams = %#v", upstreams)
	}
}
