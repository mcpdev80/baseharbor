package workload

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFromComposeReferenceDemoSelectsApplicationWorkloadOnly(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "compose.yaml")
	data := []byte(`services:
  demo-app:
    build:
      context: ./demo-app
    ports:
      - "${DEMO_HTTP_PORT:-8080}:8080"
    environment:
      PORT: "8080"
      DATABASE_URL: ${DATABASE_URL:-postgres://demo:demo@postgres:5432/demo?sslmode=disable}
      APP_SECRET: ${APP_SECRET:-standalone-demo-secret}
  postgres:
    image: postgres:17-alpine
  valkey:
    image: valkey/valkey:8-alpine
  object-storage:
    image: minio/minio:latest
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	model, err := FromCompose(path, []string{"demo-app"})
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Services) != 1 {
		t.Fatalf("services = %#v", model.Services)
	}
	service := model.Services[0]
	if service.Name != "demo-app" {
		t.Fatalf("name = %q", service.Name)
	}
	if service.Build == nil || service.Build.Context != "./demo-app" {
		t.Fatalf("build = %#v", service.Build)
	}
	if !reflect.DeepEqual(service.Ports, []Port{{Container: 8080, Protocol: "tcp"}}) {
		t.Fatalf("ports = %#v", service.Ports)
	}
	if service.Environment["PORT"] != "8080" {
		t.Fatalf("environment = %#v", service.Environment)
	}
	if service.Image != "" {
		t.Fatalf("build-backed demo unexpectedly has image %q", service.Image)
	}
}
