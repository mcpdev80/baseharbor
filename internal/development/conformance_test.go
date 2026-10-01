package development_test

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/development/goadapter"
	"github.com/mcpdev80/baseharbor/internal/development/nextjsadapter"
	"github.com/mcpdev80/baseharbor/internal/development/pythonadapter"
	"github.com/mcpdev80/baseharbor/internal/development/quarkusadapter"
)

func TestReferenceAdapterConformance(t *testing.T) {
	cases := []struct {
		name    string
		adapter development.Adapter
	}{
		{name: "go", adapter: goadapter.Adapter{}},
		{name: "nextjs", adapter: nextjsadapter.Adapter{}},
		{name: "python", adapter: pythonadapter.Adapter{}},
		{name: "quarkus", adapter: quarkusadapter.Adapter{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := development.RunAdapterConformance("", development.NewApplicationRequest{
				Name:    "conformance-" + tc.name,
				Adapter: tc.adapter.Descriptor().ID,
				Capabilities: []capability.Kind{
					capability.ExposureHTTP,
					capability.SQL,
					capability.KeyValue,
					capability.DurableKeyValue,
					capability.DocumentDatabase,
					capability.MessagingQueue,
					capability.MessagingPubSub,
					capability.MessagingStream,
					capability.ObjectStorageS3,
					capability.Secrets,
					capability.TelemetryOTLP,
				},
				Secrets: []string{"APP_SECRET"},
			}, tc.adapter)
			if !report.Passed() {
				t.Fatalf("conformance failed: %#v", report)
			}
		})
	}
}
