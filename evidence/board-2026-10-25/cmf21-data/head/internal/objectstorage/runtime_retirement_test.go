package objectstorage

import (
	"bytes"
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/availability"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestRetainedSeaweedInventoryDoesNotReissueMaterialAndRejectsDefinitionChanges(t *testing.T) {
	for _, ha := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "ha"}[ha], func(t *testing.T) {
			root := t.TempDir()
			files, err := EnsureProviderFilesAt(t.Context(), serviceissuer.New(t), root, "isolated", availability.Requirement{HA: ha})
			if err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(files.Env)
			compose, _ := os.ReadFile(files.Compose)
			_, resources, err := ExistingRuntimeInventoryAt(root, "isolated")
			if err != nil || len(resources) == 0 {
				t.Fatalf("existing executable owner not observed: %v", err)
			}
			after, _ := os.ReadFile(files.Env)
			if !bytes.Equal(before, after) {
				t.Fatal("read-only inventory changed credentials")
			}
			if err := os.WriteFile(files.Compose, append(compose, []byte("  foreign-data: {}\n")...), 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := ExistingRuntimeInventoryAt(root, "isolated"); err == nil {
				t.Fatal("modified provider owner granted retirement authority")
			}
		})
	}
}
