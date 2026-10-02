package application

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestRabbitMQRuntimeFoundationIsApplicationScopedAndPersistent(t *testing.T) {
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "events",
		Environment:   "dev",
		Services: Services{
			MessagingQueue:          true,
			MessagingQueueInstances: map[string]ServiceInstance{"jobs": {}},
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}

	placement, err := ResolveProviderPlacement(m, capability.ProviderRabbitMQ)
	if err != nil {
		t.Fatal(err)
	}
	if placement.Scope != capability.ScopeApplication || placement.Ownership != capability.OwnershipBaseHarbor {
		t.Fatalf("RabbitMQ placement = %#v", placement)
	}

	compose, err := RuntimeComposeYAMLForProject(m, "bh-events")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"rabbitmq-jobs:",
		"user: \"rabbitmq\"",
		"docker.io/library/rabbitmq:4.3.6-alpine",
		"RABBITMQ_DEFAULT_USER: ${RABBITMQ_JOBS_USER}",
		"RABBITMQ_DEFAULT_PASS: ${RABBITMQ_JOBS_PASSWORD}",
		"127.0.0.1:${RABBITMQ_JOBS_HOST_PORT}:5672",
		"rabbitmq-jobs-data:/var/lib/rabbitmq",
		"name: bh-events_rabbitmq-jobs-data",
	} {
		if !strings.Contains(compose, want) {
			t.Fatalf("RabbitMQ compose missing %q:\n%s", want, compose)
		}
	}

	env, err := newRuntimeEnv(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"RABBITMQ_JOBS_USER=baseharbor",
		"RABBITMQ_JOBS_PASSWORD=",
		"RABBITMQ_JOBS_HOST_PORT=",
	} {
		if !strings.Contains(env, want) {
			t.Fatalf("RabbitMQ runtime env missing %q:\n%s", want, env)
		}
	}

	resources := ExpectedRuntimeResourcesForIdentity(m, "bh-compose", "bh-events")
	var container, volume bool
	for _, resource := range resources {
		if resource.Kind == "container" && resource.Name == "bh-compose-rabbitmq-jobs-1" {
			container = true
		}
		if resource.Kind == "volume" && resource.Name == "bh-events_rabbitmq-jobs-data" {
			volume = true
		}
	}
	if !container || !volume {
		t.Fatalf("RabbitMQ owned resources = %#v", resources)
	}
}
