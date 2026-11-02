package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestRuntimeBrokerDocsUseGatewayOnlyWhenRequested(t *testing.T) {
	dir := t.TempDir()
	for name, contents := range map[string]string{"broker.compose.yaml": "services: {}", "broker-image": "image", "broker-docs-port": "18443"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	files := application.RuntimeFiles{Dir: dir}
	manifest := application.New("broker-demo", "dev", true, false, true)
	var out bytes.Buffer
	printRuntimeBrokerDocs(&out, "ci", manifest, files)
	if !strings.Contains(out.String(), "https://127.0.0.1:18443/") {
		t.Fatalf("direct endpoint missing: %s", out.String())
	}
	manifest.Services.SQLManagementUI = true
	out.Reset()
	printRuntimeBrokerDocs(&out, "ci", manifest, files)
	if !strings.Contains(out.String(), "https://broker-demo.baha.localhost/swagger/") {
		t.Fatalf("gateway endpoint missing: %s", out.String())
	}
}
