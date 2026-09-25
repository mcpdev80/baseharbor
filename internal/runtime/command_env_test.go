package runtime

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeCommandEnvKeepsBaseHarborXDGIsolationOutOfPodman(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("BASEHARBOR_TEST_MARKER", "present")

	env := runtimeCommandEnv("/usr/bin/podman")
	for _, entry := range env {
		if strings.HasPrefix(entry, "XDG_CONFIG_HOME=") || strings.HasPrefix(entry, "XDG_DATA_HOME=") {
			t.Fatalf("Podman runtime environment leaked BaseHarbor XDG isolation: %s", entry)
		}
	}
	found := false
	for _, entry := range env {
		if entry == "BASEHARBOR_TEST_MARKER=present" {
			found = true
		}
	}
	if !found {
		t.Fatal("unrelated environment was removed")
	}
}
