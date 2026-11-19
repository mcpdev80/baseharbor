package podman

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestInspectProjectResourcesBatchesQuadletAliasInventory(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "podman.log")
	command := filepath.Join(dir, "podman")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$PODMAN_TEST_LOG"
case "$1 $2 $3" in
  'container ls -a') printf 'quadlet-db\nquadlet-cache\n' ;;
  'container ls -aq') printf 'id-db\nid-cache\n' ;;
  'container inspect --format')
    case "$4" in
      *'{{.Id}}|{{.Name}}|'*)
        printf 'id-db|/quadlet-db|proj|<no value>|db|<no value>|{"Running":false,"Status":"exited"}\n'
        printf 'id-cache|/quadlet-cache|proj|<no value>|cache|<no value>|{"Running":false,"Status":"exited"}\n'
        ;;
      *)
        printf '/quadlet-db|proj|<no value>\n/quadlet-cache|proj|<no value>\n'
        ;;
    esac
    ;;
  *) exit 9 ;;
esac
`
	if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PODMAN_TEST_LOG", logPath)
	provider := Provider{Compose: bhruntime.NewCLIBackend(command)}
	resources := []ProjectResource{
		{Kind: "container", Name: "proj-db-1"},
		{Kind: "container", Name: "proj-cache-1"},
	}
	existing, err := provider.InspectProjectResources(context.Background(), "proj", resources)
	if err != nil {
		t.Fatalf("InspectProjectResources() error = %v", err)
	}
	if len(existing) != len(resources) {
		t.Fatalf("InspectProjectResources() returned %v, want both requested resources", existing)
	}

	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(log)), "\n")
	var inventoryLists, inventoryInspects, ownershipBatches int
	for _, line := range lines {
		if line == "container ls -aq" {
			inventoryLists++
		}
		if strings.HasPrefix(line, "container inspect --format {{.Id}}|") {
			inventoryInspects++
		}
		if strings.HasPrefix(line, "container inspect --format {{.Name}}|") {
			ownershipBatches++
			if !strings.Contains(line, "quadlet-db quadlet-cache") {
				t.Errorf("ownership inspection did not include both resolved names: %q", line)
			}
		}
	}
	if inventoryLists != 1 || inventoryInspects != 1 || ownershipBatches != 1 {
		t.Fatalf("inventory/list/ownership commands = %d/%d/%d, want 1/1/1; log:\n%s", inventoryLists, inventoryInspects, ownershipBatches, log)
	}
}
