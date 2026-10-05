package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	"go.yaml.in/yaml/v3"
)

func TestControlPlaneDefaultSingleAndExplicitHAPreserveProfileThroughTLS(t *testing.T) {
	for _, ha := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "ha"}[ha], func(t *testing.T) {
			dir := t.TempDir()
			files, err := EnsureFilesForProjectAndResources(dir, "operator", "resource", Ports{Postgres: 15432, OpenBao: 18200}, ha)
			if err != nil {
				t.Fatal(err)
			}
			check := func() {
				t.Helper()
				raw, err := os.ReadFile(files.Compose)
				if err != nil {
					t.Fatal(err)
				}
				var spec struct {
					Services map[string]any `yaml:"services"`
					Volumes  map[string]any `yaml:"volumes"`
				}
				if err := yaml.Unmarshal(raw, &spec); err != nil {
					t.Fatal(err)
				}
				wanted := 7
				volumes := 1
				if ha {
					wanted = 14
					volumes = 6
				}
				if len(spec.Services) != wanted || len(spec.Volumes) != volumes {
					t.Fatalf("ha=%t: services=%d volumes=%d", ha, len(spec.Services), len(spec.Volumes))
				}
				names, err := ControlPlaneStartupServices(ha)
				if err != nil || len(names) != wanted {
					t.Fatalf("startup resource plan: %v %v", names, err)
				}
				config, err := os.ReadFile(filepath.Join(dir, "providers", "openbao", "runtime", "openbao.hcl"))
				if err != nil {
					t.Fatal(err)
				}
				expected := `ha_enabled          = "false"`
				if ha {
					expected = `ha_enabled          = "true"`
				}
				if !strings.Contains(string(config), expected) {
					t.Fatal("OpenBao storage profile differs from runtime topology")
				}
				for _, service := range []string{"postgresql", "openbao"} {
					cfg, err := os.ReadFile(filepath.Join(dir, "providers", service, "runtime", "haproxy.cfg"))
					if err != nil {
						t.Fatal(err)
					}
					if !ha && (strings.Contains(string(cfg), "member-2") || strings.Contains(string(cfg), "port 8008")) {
						t.Fatalf("single profile retained HA proxy: %s", cfg)
					}
				}
			}
			check()
			existing, err := ExistingFilesForProject(dir, "operator")
			if err != nil || existing.HA != ha {
				t.Fatalf("persistent profile: %#v %v", existing, err)
			}
			if err := EnsureServiceAccess(context.Background(), serviceissuer.New(t), files); err != nil {
				t.Fatal(err)
			}
			check()
			before, _ := os.ReadFile(files.Compose)
			if _, err := EnsureFilesForProjectAndResources(dir, "operator", "resource", Ports{Postgres: 15432, OpenBao: 18200}, !ha); err == nil {
				t.Fatal("profile change silently accepted")
			}
			after, _ := os.ReadFile(files.Compose)
			if string(before) != string(after) {
				t.Fatal("profile conflict mutated runtime")
			}
		})
	}
}

func TestControlPlaneRefusesMissingPreFreezeProfileAndSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, composeName), []byte("old runtime"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureFiles(dir); err == nil {
		t.Fatal("pre-freeze state migrated")
	}
	original, _ := os.ReadFile(filepath.Join(dir, composeName))
	if string(original) != "old runtime" {
		t.Fatal("pre-freeze state overwritten")
	}
	outside := filepath.Join(t.TempDir(), "profile.json")
	os.WriteFile(outside, []byte(`{"version":1,"ha":false}`), 0600)
	os.Symlink(outside, filepath.Join(dir, "topology.json"))
	if _, err := readControlPlaneProfile(dir); err == nil {
		t.Fatal("symlink profile accepted")
	}
}
