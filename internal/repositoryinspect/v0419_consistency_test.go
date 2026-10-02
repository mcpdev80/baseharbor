package repositoryinspect

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestCapabilityIntentsFromManifestCoversPortableApplicationContract(t *testing.T) {
	m := application.Manifest{
		Version:       application.CurrentVersion,
		ApplicationID: application.MustNewApplicationID(),
		Name:          "all-capabilities",
		Environment:   "dev",
		Services: application.Services{
			SQL:               true,
			Cache:             true,
			KeyValue:          true,
			DocumentDatabase:  true,
			MessagingQueue:    true,
			MessagingPubSub:   true,
			MessagingStream:   true,
			ObjectStorage:     true,
			Secrets:           true,
			Identity:          true,
			CacheInstances:    map[string]application.ServiceInstance{"cache": {}},
			KeyValueInstances: map[string]application.ServiceInstance{"durable": {}},
		},
		Secrets:   application.SecretRequirements{Required: []application.SecretRequirement{{Name: "APP_SECRET"}}},
		Exposures: []application.HTTPExposureRequirement{{Name: "web", Service: "app", Port: 8080, Protocol: "http", Visibility: "public"}},
		Telemetry: application.TelemetryRequirements{OTLP: &application.OTLPRequirement{Signals: []string{"traces", "metrics", "logs"}}},
		Metrics:   application.MetricsRequirements{Sources: []application.MetricsSourceRequirement{{Name: "app", Service: "app", Port: 8080, Path: "/metrics"}}},
		Logs:      application.LogsRequirements{Collect: []string{"application"}},
		Workload:  application.WorkloadConfig{Compose: "compose.yaml", Services: []string{"app"}},
	}
	intents, err := CapabilityIntentsFromManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, intent := range intents {
		got[intent.Capability] = true
	}
	for _, want := range []string{
		"database.sql", "cache.key-value", "database.key-value", "database.document",
		"messaging.queue", "messaging.pubsub", "messaging.stream",
		"object-storage.s3", "secrets", "identity.oidc", "exposure.http",
		"telemetry.otlp", "metrics", "logs",
	} {
		if !got[want] {
			t.Fatalf("portable capability %q missing from normalized inspection intents: %#v", want, intents)
		}
	}
}

func TestComposeAnalysisClassifiesMongoDBAndRabbitMQAsInfrastructure(t *testing.T) {
	root := t.TempDir()
	data := []byte(`services:
  app:
    image: example/app:latest
  documents:
    image: mongo:7
  events:
    image: rabbitmq:4-management
`)
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	analysis, err := AnalyzeComposeFile(root, "compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(analysis.InfrastructureServices, "documents") ||
		!slices.Contains(analysis.InfrastructureServices, "events") {
		t.Fatalf("infrastructure services = %#v", analysis.InfrastructureServices)
	}
	if !slices.Contains(analysis.DocumentDatabaseInstances, "documents") {
		t.Fatalf("document database instances = %#v", analysis.DocumentDatabaseInstances)
	}
	if !slices.Contains(analysis.MessagingServices, "events") {
		t.Fatalf("messaging services = %#v", analysis.MessagingServices)
	}
	if slices.Contains(analysis.WorkloadServices, "documents") || slices.Contains(analysis.WorkloadServices, "events") {
		t.Fatalf("provider infrastructure leaked into workload classification: %#v", analysis.WorkloadServices)
	}
}
