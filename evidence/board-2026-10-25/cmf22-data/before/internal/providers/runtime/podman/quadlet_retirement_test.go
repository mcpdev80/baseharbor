package podman

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func retirementUnitFixture(t *testing.T, dir string) string {
	t.Helper()
	content := "[Unit]\nDescription=BaseHarbor Quadlet service bh-isolated-app-dev/broker\n\n[Container]\nImage=broker:test\nContainerName=bh-isolated-app-dev-broker\nLabel=com.docker.compose.project=bh-isolated-app-dev\nLabel=com.docker.compose.service=broker\nLabel=io.podman.compose.project=bh-isolated-app-dev\nLabel=io.podman.compose.service=broker\n\n[Install]\nWantedBy=default.target\n"
	path := filepath.Join(dir, "bh-isolated-app-dev-broker.container")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRetiredQuadletDefinitionCannotReappearAtBootAndKeepsSiblingData(t *testing.T) {
	dir := t.TempDir()
	path := retirementUnitFixture(t, dir)
	siblings := []string{"bh-isolated-app-dev-web.container", "bh-isolated-app-dev-operations.volume", "bh-isolated-app-dev-backend.network", "bh-isolated-app-dev-broker.env"}
	for _, name := range siblings {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("retained material"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	definitions, err := plannedQuadletRetirement(dir, "bh-isolated-app-dev", []ProjectResource{{Kind: "container", Name: "bh-isolated-app-dev-broker-1"}})
	if err != nil || len(definitions) != 1 {
		t.Fatalf("selected definitions: %+v %v", definitions, err)
	}
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	if err := os.WriteFile(filepath.Join(bin, "systemctl"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$RETIREMENT_TEST_LOG\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("RETIREMENT_TEST_LOG", log)
	if err := removeRetiredQuadletDefinitions(context.Background(), definitions); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("boot-enabled retired container definition remains")
	}
	for _, name := range siblings {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(data) != "retained material" {
			t.Fatalf("sibling material lost: %s %v", name, err)
		}
	}
	calls, err := os.ReadFile(log)
	if err != nil || string(calls) != "--user stop bh-isolated-app-dev-broker.service\n--user daemon-reload\n" {
		t.Fatalf("unexpected service authority: %q %v", calls, err)
	}
	definitions, err = plannedQuadletRetirement(dir, "bh-isolated-app-dev", []ProjectResource{{Kind: "container", Name: "bh-isolated-app-dev-broker-1"}})
	if err != nil || len(definitions) != 0 {
		t.Fatalf("retired definition cannot retry: %+v %v", definitions, err)
	}
}

func TestQuadletRetirementRejectsForeignModifiedOrReplacedOwner(t *testing.T) {
	for _, mode := range []string{"foreign", "duplicate", "symlink", "writable", "replaced"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			path := retirementUnitFixture(t, dir)
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "foreign" {
				original = []byte(strings.ReplaceAll(string(original), "bh-isolated-app-dev", "foreign-project"))
				if err := os.WriteFile(path, original, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "duplicate" {
				if err := os.WriteFile(path, append(original, []byte("ContainerName=foreign\n")...), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "symlink" {
				foreign := filepath.Join(dir, "foreign")
				if err := os.Rename(path, foreign); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(foreign, path); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "writable" {
				if err := os.Chmod(path, 0666); err != nil {
					t.Fatal(err)
				}
			}
			definitions, err := plannedQuadletRetirement(dir, "bh-isolated-app-dev", []ProjectResource{{Kind: "container", Name: "bh-isolated-app-dev-broker-1"}})
			if mode != "replaced" {
				if err == nil {
					t.Fatal("ambiguous definition accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(path, path+".retained"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("foreign replacement"), 0644); err != nil {
				t.Fatal(err)
			}
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, "systemctl"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			if err := removeRetiredQuadletDefinitions(context.Background(), definitions); err == nil {
				t.Fatal("changed owner deleted")
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != "foreign replacement" {
				t.Fatal("foreign replacement lost")
			}
		})
	}
}
