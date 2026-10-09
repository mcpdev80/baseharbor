package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
)

// The opt-in gate exercises the real CLI lifecycle and native ownership
// inventory, using isolated state and uniquely named engine resources only.
func TestBugs858860RuntimeAcceptance(t *testing.T) {
	engine := os.Getenv("BASEHARBOR_BUGFIX_RUNTIME")
	if engine == "" {
		t.Skip("isolated Docker/Podman gate")
	}
	if engine != "docker" && engine != "podman" {
		t.Fatal("invalid engine")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	path, err := exec.LookPath(engine)
	if err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.CommandContext(ctx, path, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v: %s", engine, args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	t.Logf("engine: %s", run("version", "--format", "{{.Client.Version}}"))
	if engine == "podman" {
		// BaseHarbor's XDG isolation must not relocate Podman's image/overlay
		// storage into the state snapshot or create a different engine per case.
		graph := run("info", "--format", "{{.Store.GraphRoot}}")
		runroot := run("info", "--format", "{{.Store.RunRoot}}")
		driver := run("info", "--format", "{{.Store.GraphDriverName}}")
		storage := filepath.Join(t.TempDir(), "storage.conf")
		if err := os.WriteFile(storage, []byte(fmt.Sprintf("[storage]\ndriver = %q\ngraphroot = %q\nrunroot = %q\n", driver, graph, runroot)), 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("CONTAINERS_STORAGE_CONF", storage)
	}

	run("pull", "docker.io/library/alpine:3.23")
	for _, mode := range []string{"configured", "core-present", "deployed-workload", "partial-core", "partial-secret-core", "orphan-gateway"} {
		t.Run(mode, func(t *testing.T) {
			target := configureTestTarget(t)
			t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
			cfg, err := deployment.LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			definition := cfg.Targets[target.Name]
			definition.Runtime.Provider = engine
			cfg.Targets[target.Name] = definition
			if err := cfg.Save(); err != nil {
				t.Fatal(err)
			}
			target, err = cfg.ResolveTarget("", "")
			if err != nil {
				t.Fatal(err)
			}
			m, repo := bugApplicationRepository(t)
			if mode == "configured" || mode == "partial-secret-core" {
				m.Services.Secrets = true
				if err := os.WriteFile(filepath.Join(repo, application.RepositoryManifestName), []byte(m.YAML()), 0600); err != nil {
					t.Fatal(err)
				}
			}
			t.Chdir(repo)
			var out bytes.Buffer
			command := func(args ...string) error {
				out.Reset()
				err := runWithIO(ctx, args, &out, &out)
				t.Logf("CLI %v: %v\n%s", args, err, out.String())
				return err
			}
			if err := command("--no-input", "app", "init", "--yes"); err != nil {
				t.Fatal(err)
			}
			record, found, err := deployment.FindDeployment(target.Name, m.ApplicationID, m.Environment)
			if err != nil || !found {
				t.Fatalf("normal init did not register: %v %v", found, err)
			}
			resolved, err := resolveApplication(ctx, application.DefaultStore(), nil, "destroy")
			if err != nil {
				t.Fatal(err)
			}
			files := application.RuntimeFilesFor(resolved.Store, m)
			project := application.WorkloadProjectNameForRuntime(m, files)
			suffix := fmt.Sprintf("bh-bug858-%d", time.Now().UnixNano())
			foreignNetwork := suffix + "-observability"
			volume := suffix + "-shared"
			recovery := suffix + "-recovery"
			foreignContainer := suffix + "-foreign"
			appContainer := suffix + "-app"
			t.Cleanup(func() {
				for _, name := range []string{appContainer, foreignContainer, suffix + "-gateway"} {
					_ = exec.Command(path, "container", "rm", "-f", name).Run()
				}
				_ = exec.Command(path, "network", "rm", foreignNetwork).Run()
				for _, name := range []string{volume, recovery} {
					_ = exec.Command(path, "volume", "rm", name).Run()
				}
			})
			run("network", "create", "--label", "com.docker.compose.project="+suffix, foreignNetwork)
			run("volume", "create", "--label", "com.docker.compose.project="+suffix, volume)
			run("volume", "create", "--label", "com.docker.compose.project="+project, "--label", "io.baseharbor.recovery=true", recovery)
			run("run", "-d", "--name", foreignContainer, "--label", "com.docker.compose.project="+suffix, "--label", "io.podman.compose.project="+suffix, "--label", "com.docker.compose.service=foreign", "-v", volume+":/shared", "docker.io/library/alpine:3.23", "sleep", "600")
			if mode == "deployed-workload" {
				run("run", "-d", "--name", appContainer, "--label", "com.docker.compose.project="+project, "--label", "io.podman.compose.project="+project, "--label", "com.docker.compose.service=app", "-v", volume+":/shared", "-v", recovery+":/recovery", "docker.io/library/alpine:3.23", "sleep", "600")
			}
			if mode == "core-present" {
				root, err := targetRuntimeStateRoot(target)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(root, 0700); err != nil {
					t.Fatal(err)
				}
				for name, data := range map[string]string{"compose.yaml": "services: {}\n", "runtime.env": "", "topology.json": `{"version":1,"ha":false}`} {
					if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if strings.HasPrefix(mode, "partial-") {
				root, err := targetRuntimeStateRoot(target)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(root, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "runtime.env"), []byte("partial"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "orphan-gateway" {
				gateway, err := devgateway.FilesFor(target.Name)
				if err != nil {
					t.Fatal(err)
				}
				run("run", "-d", "--name", suffix+"-gateway", "--label", "com.docker.compose.project="+gateway.Project, "--label", "io.podman.compose.project="+gateway.Project, "--label", "com.docker.compose.service=gateway", "docker.io/library/alpine:3.23", "sleep", "600")
			}
			// Documentation-only execution must leave both host state and native
			// resources byte-for-byte/inventory-identical, even for registered apps.
			roots := []string{repo, filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "baseharbor"), filepath.Join(os.Getenv("XDG_DATA_HOME"), "baseharbor"), os.Getenv("BASEHARBOR_STATE_DIR")}
			before := snapshotBugState(t, roots...)
			containersBefore := run("container", "ls", "-a", "--format", "{{.Names}}")
			for i := 0; i < 2; i++ {
				if err := command("--no-input", "app", "init", "--agents"); err != nil {
					t.Fatal(err)
				}
			}
			if !reflect.DeepEqual(before, snapshotBugState(t, roots...)) || containersBefore != run("container", "ls", "-a", "--format", "{{.Names}}") {
				t.Fatal("AGENTS-only touched runtime state/resources")
			}
			err = command("--no-input", "app", "destroy", "--yes")
			blocked := mode == "partial-secret-core" || mode == "orphan-gateway"
			if (err != nil) != blocked {
				t.Fatalf("destroy: expected blocked=%v got %v", blocked, err)
			}
			_, found, findErr := deployment.FindDeployment(target.Name, m.ApplicationID, m.Environment)
			if findErr != nil || found != blocked {
				t.Fatalf("record outcome: found=%v err=%v record=%+v", found, findErr, record.Identity)
			}
			if !blocked {
				if err := command("--no-input", "app", "destroy", "--yes"); err != nil {
					t.Fatal("repeat destroy", err)
				}
			}
			run("network", "inspect", foreignNetwork)
			run("volume", "inspect", volume)
			run("volume", "inspect", recovery)
			run("container", "inspect", foreignContainer)
			if mode == "deployed-workload" {
				if strings.Contains(run("container", "ls", "-a", "--format", "{{.Names}}"), appContainer) {
					t.Fatal("owned deployed workload remains")
				}
				if !strings.Contains(out.String(), "PRESERVED volume "+recovery) || !strings.Contains(out.String(), "PRESERVED volume "+volume) {
					t.Fatal("preserved volumes not listed", out.String())
				}
			}
			root, err := targetRuntimeStateRoot(target)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "configured" {
				if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("Core was materialized", err)
				}
			}
			t.Logf("PASS: %s; record removed=%v; foreign observability network, shared consumer, shared volume and recovery volume preserved", mode, !blocked)
		})
	}
}
