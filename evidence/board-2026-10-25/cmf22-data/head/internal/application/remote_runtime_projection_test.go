package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func remoteRuntimeProjectionFixture(t *testing.T) (RuntimeFiles, Manifest) {
	t.Helper()
	useApplicationScopedDataProviders(t)
	m := New("owned", "dev", true, false, false)
	files, err := EnsureRuntime(context.Background(), serviceissuer.New(t), Store{Root: filepath.Join(t.TempDir(), "apps")}, m)
	if err != nil {
		t.Fatal(err)
	}
	return files, m
}

func TestManagedRuntimeProjectionPreservesSQLTLSAndAuthoritativeName(t *testing.T) {
	files, m := remoteRuntimeProjectionFixture(t)
	before, _ := os.ReadFile(files.Compose)
	if err := os.WriteFile(filepath.Join(files.Dir, "unreferenced-manager.json"), []byte("forbidden-manager-fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	projection, err := ProjectManagedRuntime(files, m)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	modes := map[string]uint32{}
	for _, file := range projection.Files {
		if filepath.IsAbs(file.Path) || strings.Contains(string(file.Data), "forbidden-manager-fixture") {
			t.Fatal("projection copied an unrelated installation file")
		}
		seen[file.Path] = string(file.Data)
		modes[file.Path] = file.Mode
	}
	if !strings.HasPrefix(seen["compose.yaml"], "name: "+files.Project+"\n") || !strings.Contains(seen["runtime.env"], "POSTGRES_PASSWORD=") {
		t.Fatal("projection lost protected provider contract")
	}
	for _, member := range []string{"server-cert.pem", "server-key.pem", "pg_hba.conf"} {
		if seen["providers/postgresql/default/runtime/"+member] == "" {
			t.Fatal("projection omitted native SQL TLS file", member)
		}
	}
	if seen["bindings/postgres/default/ca.pem"] == "" {
		t.Fatal("projection omitted SQL client CA binding")
	}
	if modes["runtime.env"] != 0600 || modes["compose.yaml"] != 0600 || modes["providers/postgresql/default/runtime/server-key.pem"] != 0644 {
		t.Fatal("protected environment or native readable TLS projection permissions changed", modes)
	}
	after, _ := os.ReadFile(files.Compose)
	if string(before) != string(after) {
		t.Fatal("projection changed managed Core source definition")
	}
}

func TestManagedRuntimeProjectionIncludesCacheTLSGateway(t *testing.T) {
	useApplicationScopedDataProviders(t)
	m := New("owned", "dev", true, true, false)
	files, err := EnsureRuntime(context.Background(), serviceissuer.New(t), Store{Root: filepath.Join(t.TempDir(), "apps")}, m)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := ProjectManagedRuntime(files, m)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, file := range projection.Files {
		seen[file.Path] = true
	}
	for _, member := range []string{"providers/valkey/default/service-access/haproxy.cfg", "bindings/valkey/default/ca.pem"} {
		if !seen[member] {
			t.Fatal("projection omitted cache TLS gateway binding", member)
		}
	}
}

func TestManagedRuntimeProjectionRejectsModifiedDefinitionAndSymlinks(t *testing.T) {
	for _, kind := range []string{"definition", "leaf-symlink", "parent-symlink", "selection", "directory-permissions", "environment-permissions", "writable-bind"} {
		t.Run(kind, func(t *testing.T) {
			files, m := remoteRuntimeProjectionFixture(t)
			switch kind {
			case "environment-permissions":
				if err := os.Chmod(files.Env, 0644); err != nil {
					t.Fatal(err)
				}
			case "writable-bind":
				if err := os.Chmod(filepath.Join(files.Dir, "providers/postgresql/default/runtime/server-key.pem"), 0666); err != nil {
					t.Fatal(err)
				}
			case "definition":
				if err := os.WriteFile(files.Compose, []byte("services: {foreign: {image: alpine}}"), 0600); err != nil {
					t.Fatal(err)
				}
			case "selection":
				files.Compose = filepath.Join(t.TempDir(), "foreign.yaml")
			case "directory-permissions":
				if err := os.Chmod(files.Dir, 0755); err != nil {
					t.Fatal(err)
				}
			case "leaf-symlink":
				member := filepath.Join(files.Dir, "providers/postgresql/default/runtime/server-key.pem")
				foreign := filepath.Join(t.TempDir(), "foreign-key.pem")
				if err := os.WriteFile(foreign, []byte("foreign-key-fixture"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(member); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(foreign, member); err != nil {
					t.Fatal(err)
				}
			case "parent-symlink":
				member := filepath.Join(files.Dir, "providers/postgresql/default/runtime")
				moved := member + "-moved"
				if err := os.Rename(member, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(moved, member); err != nil {
					t.Fatal(err)
				}
			}
			projection, err := ProjectManagedRuntime(files, m)
			if err == nil || len(projection.Files) != 0 {
				t.Fatal("untrusted runtime projection admitted")
			}
			if kind == "definition" && !errors.Is(err, ErrRuntimeDefinitionChanged) {
				t.Fatal("modified runtime definition lost fail-closed classification", err)
			}
		})
	}
}
