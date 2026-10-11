package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestControlPlaneModuleInventoryIsCanonicalReadOnlyAndSeparatesSQL(t *testing.T) {
	for _, ha := range []bool{false, true} {
		root := filepath.Join(t.TempDir(), "runtime")
		files, err := EnsureFilesForProjectAndResources(root, SharedProjectName("isolated"), SharedResourceProjectName("isolated"), Ports{Postgres: 5432, OpenBao: 8200}, ha)
		if err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(files.Env)
		if err != nil {
			t.Fatal(err)
		}
		inventory, err := ExistingControlPlaneRuntimeInventory(root, files.Project)
		if err != nil || len(inventory.SQLServices) == 0 || len(inventory.SecretsServices) == 0 {
			t.Fatalf("canonical module inventory: %+v %v", inventory, err)
		}
		after, err := os.ReadFile(files.Env)
		if err != nil || string(after) != string(before) {
			t.Fatal("read inventory changed credentials")
		}
		actual, err := os.ReadFile(files.Compose)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(files.Compose, append(actual, []byte("# modified owner definition\n")...), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := ExistingControlPlaneRuntimeInventory(root, files.Project); err == nil {
			t.Fatal("noncanonical native owner definition accepted")
		}
	}
}
