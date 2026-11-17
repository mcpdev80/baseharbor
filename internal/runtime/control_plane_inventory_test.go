package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"go.yaml.in/yaml/v3"
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
		continuous := inventory.ContinuousServices(inventory.SQLServices)
		if !slices.Contains(inventory.OneShotServices, "postgres-init") || slices.Contains(continuous, "postgres-init") || !slices.Contains(inventory.SQLServices, "postgres-init") {
			t.Fatal("canonical successful init task must be separate from continuous availability and retained for lifecycle")
		}
		if slices.Contains(continuous, "postgres-etcd-recovery") {
			t.Fatal("profile-only recovery helper is required for normal SQL availability")
		}
		for _, member := range files.PostgresMembers() {
			if !slices.Contains(continuous, member) {
				t.Fatalf("required SQL member omitted: %s", member)
			}
		}
		if ha {
			if !slices.Contains(inventory.SQLServices, "postgres-etcd-recovery") {
				t.Fatal("recovery helper lost from full lifecycle service inventory")
			}
			for _, service := range []string{"postgres", "postgres-etcd-1", "postgres-etcd-2", "postgres-etcd-3"} {
				if !slices.Contains(continuous, service) {
					t.Fatalf("required SQL router/coordinator omitted: %s", service)
				}
			}
			if !slices.ContainsFunc(inventory.Resources, func(resource ProjectResource) bool {
				return resource.Kind == "container" && resource.Name == files.Project+"-postgres-etcd-recovery-1"
			}) {
				t.Fatal("recovery helper lost from complete owned resources")
			}
		}
		after, err := os.ReadFile(files.Env)
		if err != nil || string(after) != string(before) {
			t.Fatal("read inventory changed credentials")
		}
		actual, err := os.ReadFile(files.Compose)
		if err != nil {
			t.Fatal(err)
		}
		var legacyDocument map[string]any
		if err := yaml.Unmarshal(actual, &legacyDocument); err != nil {
			t.Fatal(err)
		}
		services := legacyDocument["services"].(map[string]any)
		for _, rawService := range services {
			service := rawService.(map[string]any)
			if user, ok := legacyControlPlaneUserForImage(service["image"].(string)); ok {
				service["user"] = user
			}
		}
		legacy, err := yaml.Marshal(legacyDocument)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(files.Compose, legacy, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := ExistingControlPlaneRuntimeInventory(root, files.Project); err != nil {
			t.Fatalf("known prior fixed-UID renderer must remain upgradeable: %v", err)
		}
		modified := strings.Replace(string(legacy), "restart: unless-stopped", "restart: no", 1)
		if modified == string(legacy) {
			t.Fatal("test fixture has no restart policy to mutate")
		}
		if err := os.WriteFile(files.Compose, []byte(modified), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := ExistingControlPlaneRuntimeInventory(root, files.Project); err == nil {
			t.Fatal("noncanonical native owner definition accepted")
		}
	}
}

func TestControlPlaneInventoryRejectsLinkedOrForeignRecoveryOwners(t *testing.T) {
	for _, mutation := range []string{"compose-hardlink", "env-hardlink", "linked-parent", "foreign-env-owner"} {
		t.Run(mutation, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "runtime")
			files, err := EnsureFilesForProjectAndResources(root, SharedProjectName("isolated"), SharedResourceProjectName("isolated"), Ports{Postgres: 5432, OpenBao: 8200}, false)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(files.Env)
			if err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "compose-hardlink", "env-hardlink":
				path := files.Compose
				if mutation == "env-hardlink" {
					path = files.Env
				}
				if err := os.Link(path, filepath.Join(t.TempDir(), "foreign-owner")); err != nil {
					t.Fatal(err)
				}
			case "linked-parent":
				link := filepath.Join(t.TempDir(), "redirected")
				if err := os.Symlink(root, link); err != nil {
					t.Fatal(err)
				}
				root = link
			case "foreign-env-owner":
				if os.Getuid() != 0 {
					t.Skip("changing fixture UID requires root")
				}
				if err := os.Chown(files.Env, 65534, -1); err != nil {
					if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.EPERM) {
						t.Skip("fixture namespace does not permit a foreign UID")
					}
					t.Fatal(err)
				}
			}
			if _, err := ExistingControlPlaneRuntimeInventory(root, files.Project); err == nil {
				t.Fatal("foreign recovery authority accepted")
			}
			after, err := os.ReadFile(files.Env)
			if err != nil || string(before) != string(after) {
				t.Fatal("refused inventory mutated retained credentials")
			}
		})
	}
}
