package quarkusadapter

import (
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/development"
)

func TestQuarkusRoundTrip(t *testing.T) {
	registry, err := development.NewRegistry(Adapter{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := development.CreateApplication(filepath.Join(t.TempDir(), "quarkus"), development.NewApplicationRequest{
		Name: "quarkus-app", Adapter: AdapterID,
		Capabilities: []capability.Kind{
			capability.ExposureHTTP, capability.SQL, capability.KeyValue, capability.DurableKeyValue,
			capability.DocumentDatabase, capability.MessagingQueue, capability.MessagingPubSub, capability.MessagingStream,
			capability.ObjectStorageS3, capability.Secrets, capability.TelemetryOTLP,
		},
	}, registry)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Validation.Satisfied {
		t.Fatalf("validation = %#v", result.Validation)
	}
}
