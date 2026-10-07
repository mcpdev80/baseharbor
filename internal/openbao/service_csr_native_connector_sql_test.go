package openbao

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/runtime/remoteprojection"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// This qualifies protected generated SQL material on an actual enrolled native
// node. It is deliberately not a full Application HTTP lifecycle receipt.
func (f *nativeConnectorFixture) sqlProject(t *testing.T, ctx context.Context, pool *targetsession.Pool, scope targetenrollment.Scope, issuer *ServiceIssuer) {
	t.Helper()
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication))
	m := application.New("remote-sql-"+f.name[len(f.name)-16:], "dev", true, false, false)
	files, err := application.EnsureRuntime(ctx, issuer, application.Store{Root: filepath.Join(f.dir, "sql-apps")}, m)
	if err != nil {
		t.Fatal("Core-generated managed SQL files failed", err)
	}
	projection, err := application.ProjectManagedRuntime(files, m)
	if err != nil {
		t.Fatal("protected SQL projection failed", err)
	}
	var source []targetsession.ProjectFile
	for _, file := range projection.Files {
		source = append(source, targetsession.ProjectFile{Path: file.Path, Data: file.Data, Mode: file.Mode})
	}
	var quadletUnits []string
	if f.engine == "podman" {
		var members []string
		for _, file := range projection.Files {
			members = append(members, file.Path)
		}
		graph, err := remoteprojection.ProjectRemoteQuadletGraph(files.Compose, files.Env, projection.Project, members)
		if err != nil {
			t.Fatal("Core SQL Quadlet graph projection failed", err)
		}
		quadletUnits = graph.Units
		var names []string
		for name := range graph.Files {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			source = append(source, targetsession.ProjectFile{Path: name, Data: []byte(graph.Files[name]), Mode: 0600})
		}
	}
	runtime, err := targetsession.NewProjectRuntime(pool, scope)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := runtime.Stage(ctx, projection.Project, source)
	if err != nil {
		t.Fatal("SQL project staging failed", err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// Emergency cleanup cannot establish success. Successful removal below
		// is asserted using the authenticated Core adapter and native inventory.
		if f.engine == "docker" {
			_ = exec.CommandContext(cleanup, "docker", "compose", "-p", projection.Project, "-f", files.Compose, "--env-file", files.Env, "down", "--volumes").Run()
		} else {
			root := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "containers", "systemd")
			for _, file := range quadletUnits {
				base, ext := strings.TrimSuffix(file, filepath.Ext(file)), filepath.Ext(file)
				if ext != ".container" {
					base += "-" + strings.TrimPrefix(ext, ".")
				}
				_ = exec.CommandContext(cleanup, "systemctl", "--user", "stop", base+".service").Run()
				_ = os.Remove(filepath.Join(root, file))
				_ = os.RemoveAll(filepath.Join(root, file+".d"))
			}
			_ = exec.CommandContext(cleanup, "systemctl", "--user", "daemon-reload").Run()
			for _, kind := range []string{"volume", "network"} {
				output, err := exec.CommandContext(cleanup, "podman", kind, "ls", "-q", "--filter", "label=com.docker.compose.project="+projection.Project).Output()
				if err != nil {
					t.Error("SQL fixture resource cleanup inventory failed", kind)
					continue
				}
				for _, name := range strings.Fields(string(output)) {
					if err := exec.CommandContext(cleanup, "podman", kind, "rm", name).Run(); err != nil {
						t.Error("SQL fixture resource cleanup failed", kind)
					}
				}
			}
		}
	})
	verify := func() {
		t.Helper()
		deadline := time.Now().Add(45 * time.Second)
		probe := application.NewRemoteBackendProbeExecutor(runtime, projection.Project)
		for {
			err = application.VerifyPostgresProvider(ctx, probe, m)
			if err == nil {
				break
			}
			if time.Now().After(deadline) || ctx.Err() != nil {
				t.Fatal("actual enrolled SQL TLS SELECT 1 did not become ready", err)
			}
			time.Sleep(250 * time.Millisecond)
		}
		out, err := runtime.ExecService(ctx, projection.Project, "postgres", "id", "-u")
		if err != nil || strings.TrimSpace(out) != "70" {
			t.Fatal("generated SQL did not run as its unprivileged native user", err)
		}
	}
	apply := func(repair bool) error {
		if f.engine == "podman" {
			return runtime.ApplyQuadletGraph(ctx, staged, quadletUnits)
		}
		return runtime.ApplyCompose(ctx, staged, []string{projection.Compose}, projection.Env, repair)
	}
	if err := apply(false); err != nil {
		t.Fatal("generated SQL apply failed", err)
	}
	verify()
	record := persistNativeSQLProject(t, f.dir, m, staged.Record())
	runtime, err = targetsession.NewProjectRuntime(pool, scope)
	if err != nil {
		t.Fatal(err)
	}
	staged, err = runtime.RestoreProject(record, source)
	if err != nil {
		t.Fatal("SQL protected receipt restoration failed", err)
	}
	if err := apply(true); err != nil {
		t.Fatal("generated SQL repair failed", err)
	}
	verify()
	t.Log("protected deployment registry round-trip restored the same immutable SQL project without restaging; Application lifecycle not qualified")
	retainedVolume := ""
	if f.engine == "podman" {
		retainedVolume = f.sqlVolumeIdentity(t, ctx, projection.Project)
	}
	if f.engine == "podman" {
		err = runtime.DestroyQuadletGraph(ctx, staged, quadletUnits)
	} else {
		err = runtime.DestroyComposeOwned(ctx, staged, []string{projection.Compose}, projection.Env, true)
	}
	if err != nil {
		t.Fatal("owned SQL reset failed", err)
	}
	services, err := runtime.ObserveProject(ctx, projection.Project)
	if err != nil || len(services) != 0 {
		t.Fatal("owned SQL containers survived destroy", err)
	}
	if f.engine == "docker" {
		for _, kind := range []string{"volume", "network"} {
			out, err := exec.CommandContext(ctx, "docker", kind, "ls", "-q", "--filter", "label=com.docker.compose.project="+projection.Project).Output()
			if err != nil || strings.TrimSpace(string(out)) != "" {
				t.Fatal("owned SQL persistent resources survived explicit reset", kind, err)
			}
		}
	} else {
		f.reapplyRetainedSQL(t, ctx, runtime, staged, quadletUnits, projection.Project, retainedVolume, verify)
	}
	if f.inventory(t, ctx, pool, scope) == "" {
		t.Fatal("SQL reset damaged the foreign fixture")
	}
	t.Log("managed generated SQL project preserved protected TLS material, native UID 70, verified TLS SELECT 1 and immutable repair; owned containers destroyed and foreign fixture preserved; Application lifecycle not qualified")
}
