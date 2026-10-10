package deployment

import (
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"os"
	"path/filepath"
	"testing"
)

func TestBug865StateOverrideIsolatesConfigurationAndDeployment(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	foreign := Config{Version: ConfigVersion, Prompt: PromptConfig{Enabled: true, Preset: "detailed"}}
	if err := foreign.Save(); err != nil {
		t.Fatal(err)
	}
	foreignPath, _ := ConfigPath()
	before, _ := os.ReadFile(foreignPath)
	id := testDeploymentIdentity(t, "local", "foreign", "dev")
	if err := SaveDeploymentRecord(DeploymentRecord{Identity: id, Applied: AppliedDeployment{RuntimeProvider: "docker"}}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	data, err := DataRoot()
	if err != nil || data != root {
		t.Fatalf("data override %s %v", data, err)
	}
	runtime, err := bhruntime.DataDir("")
	if err != nil || runtime != data {
		t.Fatalf("runtime split brain %s %v", runtime, err)
	}
	for _, xdg := range []string{t.TempDir(), ""} {
		t.Setenv("XDG_DATA_HOME", xdg)
		t.Setenv("XDG_CONFIG_HOME", xdg)
		records, err := ListDeployments("local")
		if err != nil || len(records) != 0 {
			t.Fatalf("foreign deployments visible %v %v", records, err)
		}
		cfg, err := LoadConfig()
		if err != nil || cfg.Prompt.Enabled {
			t.Fatal("foreign config visible", err)
		}
		for _, target := range []string{"local", "other"} {
			path, err := TargetStateRoot(target)
			if err != nil || path != filepath.Join(root, "targets", target) {
				t.Fatalf("target override %s %v", path, err)
			}
		}
	}
	if err := (Config{Version: ConfigVersion, Prompt: PromptConfig{Preset: "compact"}}).Save(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(foreignPath)
	if string(before) != string(after) {
		t.Fatal("foreign config changed")
	}
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	// Restoring the original XDG paths is checked by the foreign file above;
	// an independent override must not discover the first installation.
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	cfg, err := LoadConfig()
	if err != nil || cfg.Prompt.Preset != "" {
		t.Fatal("isolated config crossed override", err)
	}
}
