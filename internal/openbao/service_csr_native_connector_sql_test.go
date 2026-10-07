package openbao

import (
	"context"
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

// This qualifies protected generated SQL material on an actual enrolled Docker
// node. It is deliberately not a full Application HTTP lifecycle receipt.
func (f *nativeConnectorFixture) sqlProject(t *testing.T, ctx context.Context, pool *targetsession.Pool, scope targetenrollment.Scope, issuer *ServiceIssuer) {
	t.Helper()
	if f.engine != "docker" {
		return
	}
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
		_ = exec.CommandContext(cleanup, "docker", "compose", "-p", projection.Project, "-f", files.Compose, "--env-file", files.Env, "down", "--volumes").Run()
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
	if err := runtime.ApplyCompose(ctx, staged, []string{projection.Compose}, projection.Env, false); err != nil {
		t.Fatal("generated SQL apply failed", err)
	}
	verify()
	record := staged.Record()
	runtime, err = targetsession.NewProjectRuntime(pool, scope)
	if err != nil {
		t.Fatal(err)
	}
	staged, err = runtime.RestoreProject(record, source)
	if err != nil {
		t.Fatal("SQL protected receipt restoration failed", err)
	}
	if err := runtime.ApplyCompose(ctx, staged, []string{projection.Compose}, projection.Env, true); err != nil {
		t.Fatal("generated SQL repair failed", err)
	}
	verify()
	if err := runtime.DestroyComposeOwned(ctx, staged, []string{projection.Compose}, projection.Env, true); err != nil {
		t.Fatal("owned SQL reset failed", err)
	}
	services, err := runtime.ObserveProject(ctx, projection.Project)
	if err != nil || len(services) != 0 {
		t.Fatal("owned SQL containers survived destroy", err)
	}
	for _, kind := range []string{"volume", "network"} {
		out, err := exec.CommandContext(ctx, "docker", kind, "ls", "-q", "--filter", "label=com.docker.compose.project="+projection.Project).Output()
		if err != nil || strings.TrimSpace(string(out)) != "" {
			t.Fatal("owned SQL persistent resources survived explicit reset", kind, err)
		}
	}
	if f.inventory(t, ctx, pool, scope) == "" {
		t.Fatal("SQL reset damaged the foreign fixture")
	}
	t.Log("managed generated SQL project preserved protected TLS material, native UID 70, verified TLS SELECT 1, immutable repair and explicit owned reset; Application lifecycle not qualified")
}
