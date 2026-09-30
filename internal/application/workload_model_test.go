package application

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/workload"
)

func TestResolveRepositoryWorkloadModelKeepsManifestSelectionPortable(t *testing.T) {
	root := t.TempDir()
	compose := `services:
  demo-app:
    image: example/demo:latest
    ports:
      - "8080:8080"
  postgres:
    image: postgres:17-alpine
  valkey:
    image: valkey/valkey:8-alpine
`
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}

	m := Manifest{
		Version:     CurrentVersion,
		Name:        "demo",
		Environment: "dev",
		Services: Services{
			Postgres: true,
			Redis:    true,
		},
		Workload: WorkloadConfig{
			Compose:  "compose.yaml",
			Services: []string{"demo-app"},
		},
	}

	model, found, err := ResolveRepositoryWorkloadModel(root, m)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("repository workload not found")
	}
	if len(model.Services) != 1 {
		t.Fatalf("services = %#v", model.Services)
	}
	if got, want := model.Services[0].Name, "demo-app"; got != want {
		t.Fatalf("service = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(model.Services[0].Ports, []workload.Port{{Container: 8080, Protocol: "tcp"}}) {
		t.Fatalf("ports = %#v", model.Services[0].Ports)
	}
}
