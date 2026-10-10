package nextjsadapter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/development"
)

func TestNextJSRoundTrip(t *testing.T) {
	registry, err := development.NewRegistry(Adapter{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := development.CreateApplication(filepath.Join(t.TempDir(), "next"), development.NewApplicationRequest{
		Name: "next-app", Adapter: AdapterID,
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

func TestNextJSGeneratedHTTPPortAgreement(t *testing.T) {
	registry, err := development.NewRegistry(Adapter{})
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "next-app")
	if _, err := development.CreateApplication(root, development.NewApplicationRequest{
		Name: "next-app", Adapter: AdapterID, Capabilities: []capability.Kind{capability.ExposureHTTP},
	}, registry); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		path      string
		required  []string
		forbidden []string
	}{
		{"baseharbor.yaml", []string{"8080"}, []string{"port: 3000"}},
		{"compose.yaml", []string{"8080:8080", "PORT: \"8080\"", "127.0.0.1:8080/healthz"}, []string{"3000:3000", "127.0.0.1:3000"}},
		{"Dockerfile", []string{"EXPOSE 8080"}, []string{"EXPOSE 3000"}},
	} {
		body, err := os.ReadFile(filepath.Join(root, check.path))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range check.required {
			if !strings.Contains(string(body), want) {
				t.Errorf("%s missing %q", check.path, want)
			}
		}
		for _, old := range check.forbidden {
			if strings.Contains(string(body), old) {
				t.Errorf("%s still uses %q", check.path, old)
			}
		}
	}
}
