package podman

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func explorerUnitFixture(t *testing.T) (Provider, string, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dir, err := quadletUserUnitDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := retirementUnitFixture(t, dir)
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	t.Setenv("EXPLORER_UNIT_TEST_LOG", log)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for name, script := range map[string]string{
		"podman":    "#!/bin/sh\nprintf 'podman %s\\n' \"$*\" >> \"$EXPLORER_UNIT_TEST_LOG\"\ncase \"$*\" in 'container ls -aq') exit 0;; *) exit 2;; esac\n",
		"systemctl": "#!/bin/sh\nprintf 'systemctl %s\\n' \"$*\" >> \"$EXPLORER_UNIT_TEST_LOG\"\ncase \"$*\" in '--user show '*) printf 'inactive\\n';; '--user start '*|'--user stop '*|'--user restart '*) exit 0;; *) exit 2;; esac\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return Provider{Compose: bhruntime.NewCLIBackend(filepath.Join(bin, "podman"))}, path, log
}

func TestExplorerQuadletIdentitySurvivesNativeAbsenceAndUsesSupervisor(t *testing.T) {
	provider, path, log := explorerUnitFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	containers, err := provider.ExplorerRuntimeContainers(t.Context())
	if err != nil || len(containers) != 1 || containers[0].ID != "quadlet:bh-isolated-app-dev-broker" || containers[0].Running || containers[0].State != "stopped" {
		t.Fatalf("unverified unit state or lost existing identity: %+v %v", containers, err)
	}
	for _, operation := range []string{"stop", "start", "restart"} {
		if _, err := provider.ExplorerOperateContainer(t.Context(), containers[0].ID, operation); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := provider.ExplorerContainerNativeID(t.Context(), containers[0].ID); err == nil {
		t.Fatal("absent native container invented for logs/exec")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("runtime operation erased or rewrote boot/recovery owner")
	}
	calls, _ := os.ReadFile(log)
	for _, operation := range []string{"stop", "start", "restart"} {
		if !strings.Contains(string(calls), "systemctl --user "+operation+" bh-isolated-app-dev-broker.service") {
			t.Fatal("operation bypassed native supervisor")
		}
	}
	if strings.Contains(string(calls), "podman container stop") {
		t.Fatal("raw container stop bypassed Quadlet")
	}
}

func TestExplorerQuadletRejectsChangedOrLinkedOwner(t *testing.T) {
	provider, path, _ := explorerUnitFixture(t)
	proof, err := provider.ExplorerContainerEvidence(t.Context(), "quadlet:bh-isolated-app-dev-broker")
	if err != nil || proof["runtime.quadlet.revision"] == "" {
		t.Fatal("missing existing owner digest", err)
	}
	data, _ := os.ReadFile(path)
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), "ContainerName=bh-isolated-app-dev-broker", "ContainerName=foreign", 1)), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.ExplorerOperateContainer(t.Context(), "quadlet:bh-isolated-app-dev-broker", "stop"); err == nil {
		t.Fatal("foreign native identity permitted mutation")
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, filepath.Join(t.TempDir(), "foreign-link")); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.ExplorerOperateContainer(t.Context(), "quadlet:bh-isolated-app-dev-broker", "stop"); err == nil {
		t.Fatal("multi-linked unit owner granted mutation")
	}
}
