package openbao

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
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
	managed, err := application.NewRemoteManagedRuntime(pool, scope, files, m)
	if err != nil {
		t.Fatal("Core managed provider compilation failed", err)
	}
	var quadletUnits []string
	runtime, err := targetsession.NewProjectRuntime(pool, scope)
	if err != nil {
		t.Fatal(err)
	}
	capacity, err := runtime.NodeMemory(ctx)
	if err != nil || capacity.TotalBytes == 0 || capacity.AvailableBytes > capacity.TotalBytes {
		t.Fatal("fresh authenticated execution-node memory evidence failed", err)
	}
	t.Log("fresh execution-node memory evidence obtained over exact enrolled scope; Core-host fallback not used")
	var record targetsession.ProjectRecord
	snapshotState := filepath.Join(f.dir, "sql-publication-state")
	if err := os.Mkdir(snapshotState, 0700); err != nil {
		t.Fatal("protected SQL publication state creation failed", err)
	}
	if err := managed.Publish(ctx, func(project targetsession.ProjectRecord) error {
		if err := managed.SaveSnapshot(snapshotState, project); err != nil {
			return err
		}
		record = persistNativeSQLProject(t, f.dir, m, project)
		return nil
	}); err != nil {
		t.Fatal("SQL project staging failed", err)
	}
	if f.engine == "podman" {
		for _, file := range managed.Record().Files {
			ext := filepath.Ext(file.Path)
			if ext == ".container" || ext == ".network" || ext == ".volume" {
				quadletUnits = append(quadletUnits, file.Path)
			}
		}
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
		probe := application.NewRemoteBackendProbeExecutor(managed, projection.Project)
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
		out, err := managed.ExecService(ctx, projection.Project, "postgres", "id", "-u")
		if err != nil || strings.TrimSpace(out) != "70" {
			t.Fatal("generated SQL did not run as its unprivileged native user", err)
		}
	}
	apply := func(repair bool) error { return managed.Apply(ctx, repair) }
	if err := apply(false); err != nil {
		t.Fatal("generated SQL apply failed", err)
	}
	verify()
	// Change the original Core files to prove that repair restores private
	// publication bytes, rather than regenerating from today's mutable source.
	for _, file := range []string{files.Compose, files.Env} {
		original, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, append(original, []byte("\n# changed-after-publication\n")...), 0600); err != nil {
			t.Fatal(err)
		}
	}
	managed, err = application.RestoreRemoteManagedSnapshot(pool, scope, snapshotState, record)
	if err != nil {
		t.Fatal("SQL protected publication snapshot restoration failed", err)
	}
	if err := apply(true); err != nil {
		t.Fatal("generated SQL repair failed", err)
	}
	verify()
	t.Log("protected deployment registry round-trip restored the same immutable SQL project without restaging; Application lifecycle not qualified")
	t.Log("private Core publication snapshot restored SQL repair and owned teardown after original Core source changed; exact registry receipt retained; full Application engine not qualified")
	retainedVolume := ""
	if f.engine == "podman" {
		retainedVolume = f.sqlVolumeIdentity(t, ctx, projection.Project)
	}
	err = managed.DestroyOwned(ctx, f.engine == "docker")
	if err != nil {
		t.Fatal("owned SQL reset failed", err)
	}
	services, err := managed.Observe(ctx)
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
		f.reapplyRetainedSQL(t, ctx, managed, projection.Project, retainedVolume, verify)
		if err := managed.DestroyOwned(ctx, true); err != nil {
			t.Fatal("explicit owned Podman SQL reset failed", err)
		}
		if err := managed.DestroyOwned(ctx, true); err != nil {
			t.Fatal("explicit reset retry failed after verified absence", err)
		}
		for _, kind := range []string{"volume", "network"} {
			output, err := exec.CommandContext(ctx, "podman", kind, "ls", "-q", "--filter", "label=com.docker.compose.project="+projection.Project).Output()
			if err != nil || strings.TrimSpace(string(output)) != "" {
				t.Fatal("owned Podman persistent resource survived reset", kind, err)
			}
		}
		t.Log("actual enrolled Podman explicit owned SQL data reset removed exact provider volume, verified absence and reconciled retry without starting containers; foreign fixture preserved")
	}
	if f.inventory(t, ctx, pool, scope) == "" {
		t.Fatal("SQL reset damaged the foreign fixture")
	}
	t.Log("actual enrolled Core-managed provider snapshot publish-restore-apply-repair-destroy preserved immutable TLS source; full Application engine not qualified")
	t.Log("managed generated SQL project preserved protected TLS material, native UID 70, verified TLS SELECT 1 and immutable repair; owned containers destroyed and foreign fixture preserved; Application lifecycle not qualified")
}
