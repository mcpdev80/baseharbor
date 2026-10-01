package pythonadapter

import (
	"os"
	"strings"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/development"
)

func TestPythonRoundTrip(t *testing.T) {
	registry, err := development.NewRegistry(Adapter{})
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "python")
	result, err := development.CreateApplication(root, development.NewApplicationRequest{
		Name: "python-app", Adapter: AdapterID,
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
		t.Fatalf("validation=%#v", result.Validation)
	}
	if _, err := os.Stat(filepath.Join(root, "pyproject.toml")); err != nil {
		t.Fatal(err)
	}
	compose, err := os.ReadFile(filepath.Join(root, "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"healthcheck:", "/healthz", "urllib.request"} {
		if !strings.Contains(string(compose), want) {
			t.Fatalf("generated Python compose is missing readiness contract %q:\n%s", want, compose)
		}
	}
}
