package application

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestMessagingManifestRoundTripPortableContractAndPlan(t *testing.T) {
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "events",
		Environment:   "dev",
		Services: Services{
			MessagingQueue:           true,
			MessagingPubSub:          true,
			MessagingStream:          true,
			MessagingQueueInstances:  map[string]ServiceInstance{"jobs": {}},
			MessagingPubSubInstances: map[string]ServiceInstance{"updates": {}},
			MessagingStreamInstances: map[string]ServiceInstance{"events": {}},
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	rendered := m.YAML()
	for _, want := range []string{"messaging_queue:", "jobs: {}", "messaging_pubsub:", "updates: {}", "messaging_stream:", "events: {}"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("manifest missing %q:\n%s", want, rendered)
		}
	}
	got, err := ParseYAML(rendered)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := PortableContractFromManifest(got)
	if err != nil {
		t.Fatal(err)
	}
	want := map[capability.Kind]string{
		capability.MessagingQueue:  "jobs",
		capability.MessagingPubSub: "updates",
		capability.MessagingStream: "events",
	}
	for _, requirement := range contract.Capabilities {
		if name, ok := want[requirement.Kind]; ok {
			if requirement.Name != name {
				t.Fatalf("%s name = %q, want %q", requirement.Kind, requirement.Name, name)
			}
			delete(want, requirement.Kind)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing messaging capabilities: %#v", want)
	}
	resources, err := ResolveCapabilityResources(contract)
	if err != nil {
		t.Fatal(err)
	}
	var messagingResources int
	for _, resource := range resources {
		switch resource.Kind {
		case capability.MessagingQueue, capability.MessagingPubSub, capability.MessagingStream:
			messagingResources++
			if resource.Provider != capability.ProviderRabbitMQ {
				t.Fatalf("%s provider = %q", resource.Kind, resource.Provider)
			}
		}
	}
	if messagingResources != 3 {
		t.Fatalf("messaging resources = %d", messagingResources)
	}
	plan, err := BuildPlan(got)
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, action := range plan.Actions {
		joined += action.Resource + " " + action.Description + "\n"
	}
	for _, want := range []string{
		"messaging.queue:jobs", "publish/consume/ack",
		"messaging.pubsub:updates", "subscribe/publish/receive",
		"messaging.stream:events", "stream semantics",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("plan missing %q:\n%s", want, joined)
		}
	}
}
