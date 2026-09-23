package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderComposeServiceQuadletDemoShape(t *testing.T) {
	root := t.TempDir()
	compose := filepath.Join(root, "compose.yaml")
	if err := os.Mkdir(filepath.Join(root, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	data := `services:
  api:
    build:
      context: ./app
    ports:
      - "8080:8080"
    environment:
      PORT: "8080"
      DATABASE_URL: ${DATABASE_URL:-postgres://demo@postgres/demo}
    restart: unless-stopped
`
	if err := os.WriteFile(compose, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := RenderComposeServiceQuadlet(compose, "api", "baseharbor-demo")
	if err != nil {
		t.Fatal(err)
	}
	if got.UnitBase != "baseharbor-demo-api" {
		t.Fatalf("unit base = %q", got.UnitBase)
	}
	for _, want := range []string{
		"ImageTag=localhost/baseharbor-demo-api:quadlet-poc",
		"SetWorkingDirectory=" + filepath.Join(root, "app"),
		"File=Dockerfile",
	} {
		if !strings.Contains(got.Build, want) {
			t.Fatalf("build missing %q:\n%s", want, got.Build)
		}
	}
	for _, want := range []string{
		"Image=baseharbor-demo-api.build",
		"ContainerName=baseharbor-demo-api",
		"EnvironmentFile=./baseharbor-demo-api.env",
		"PublishPort=8080:8080",
		"Restart=always",
	} {
		if !strings.Contains(got.Container, want) {
			t.Fatalf("container missing %q:\n%s", want, got.Container)
		}
	}
	if !strings.Contains(got.Environment, "DATABASE_URL=postgres://demo@postgres/demo") {
		t.Fatalf("environment defaults not resolved:\n%s", got.Environment)
	}
}

func TestRenderComposeServiceQuadletRejectsUnsupportedFeatures(t *testing.T) {
	root := t.TempDir()
	compose := filepath.Join(root, "compose.yaml")
	data := `services:
  api:
    image: alpine
    volumes:
      - data:/data
`
	if err := os.WriteFile(compose, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderComposeServiceQuadlet(compose, "api", "demo"); err == nil {
		t.Fatal("expected unsupported feature failure")
	}
}
