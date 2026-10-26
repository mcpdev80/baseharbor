package capability

import "testing"

func TestMessagingSpecificationsAndRabbitMQReferenceProvider(t *testing.T) {
	cases := []struct {
		id   SpecificationID
		kind Kind
	}{
		{MessagingQueueV1.ID, MessagingQueue},
		{MessagingPubSubV1.ID, MessagingPubSub},
		{MessagingStreamV1.ID, MessagingStream},
	}
	for _, tc := range cases {
		spec, err := ParseSpecificationID(tc.id)
		if err != nil {
			t.Fatalf("parse %s: %v", tc.id, err)
		}
		if spec.Kind != tc.kind {
			t.Fatalf("%s kind = %q, want %q", tc.id, spec.Kind, tc.kind)
		}
		service, err := ServiceKindForCapability(tc.kind)
		if err != nil {
			t.Fatalf("service kind for %s: %v", tc.kind, err)
		}
		if service != ServiceMessaging {
			t.Fatalf("%s service = %q", tc.kind, service)
		}
		if !RabbitMQ.Supports(tc.kind) {
			t.Fatalf("RabbitMQ does not support %s", tc.kind)
		}
	}
	integration, err := ReferenceIntegration(ProviderRabbitMQ)
	if err != nil {
		t.Fatal(err)
	}
	if integration.Provider.Kind != ProviderRabbitMQ {
		t.Fatalf("provider = %q", integration.Provider.Kind)
	}
	if len(integration.Capabilities) != 3 {
		t.Fatalf("capabilities = %#v", integration.Capabilities)
	}
	if len(integration.Interfaces) != 2 ||
		integration.Interfaces[0].Name != "amqp" ||
		integration.Interfaces[0].Protocol != "amqp" ||
		integration.Interfaces[1].Name != "management-ui" ||
		integration.Interfaces[1].Protocol != "https" ||
		!integration.Interfaces[1].Optional {
		t.Fatalf("interfaces = %#v", integration.Interfaces)
	}
}
