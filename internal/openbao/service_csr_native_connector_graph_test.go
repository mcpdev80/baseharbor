package openbao

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// This qualifies graph publication/activation ordering on the actual enrolled
// rootless Node. It does not qualify the complete Application engine.
func (f *nativeConnectorFixture) quadletDependencyGraph(t *testing.T, ctx context.Context, pool *targetsession.Pool, scope targetenrollment.Scope) {
	t.Helper()
	runtime, err := targetsession.NewProjectRuntime(pool, scope)
	if err != nil {
		t.Fatal(err)
	}
	project := f.name + "-graph"
	appName, dependencyName := project+"-a", project+"-z"
	appFile, dependencyFile := appName+".container", dependencyName+".container"
	content := func(name, service, header string) []byte {
		return []byte(header + "[Container]\nImage=" + f.image + "\nContainerName=" + name + "\nUser=1000:1000\nReadOnly=true\nExec=sleep 300\nLabel=baseharbor.enrollment-qualification=true\nLabel=com.docker.compose.project=" + project + "\nLabel=com.docker.compose.service=" + service + "\n[Service]\nTimeoutStartSec=45\n")
	}
	source := []targetsession.ProjectFile{
		{Path: appFile, Data: content(appName, "app", "[Unit]\nRequires="+dependencyName+".service\nAfter="+dependencyName+".service\n")},
		{Path: dependencyFile, Data: content(dependencyName, "dependency", "")},
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// Emergency fixture cleanup is not evidence of successful destruction.
		root := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "containers", "systemd")
		for _, name := range []string{appName, dependencyName} {
			_ = exec.CommandContext(cleanup, "systemctl", "--user", "stop", name+".service").Run()
			_ = os.Remove(filepath.Join(root, name+".container"))
			_ = os.RemoveAll(filepath.Join(root, name+".container.d"))
			_ = exec.CommandContext(cleanup, "podman", "rm", "-f", name).Run()
		}
		_ = exec.CommandContext(cleanup, "systemctl", "--user", "daemon-reload").Run()
	})
	staged, err := runtime.Stage(ctx, project, source)
	if err != nil {
		t.Fatal("dependency graph staging failed", err)
	}
	units := []string{appFile, dependencyFile}
	verify := func() {
		t.Helper()
		services, err := runtime.ObserveProject(ctx, project)
		if err != nil || len(services) != 2 {
			t.Fatal("dependency graph inventory is incomplete", err)
		}
		starts := map[string]time.Time{}
		for _, service := range services {
			if !service.Running || service.StartedAt.IsZero() || (service.Service != "app" && service.Service != "dependency") {
				t.Fatal("dependency graph service did not converge")
			}
			starts[service.Service] = service.StartedAt
			output, err := runtime.ExecService(ctx, project, service.Service, "id", "-u")
			if err != nil || strings.TrimSpace(output) != "1000" {
				t.Fatal("graph service probe did not execute in owned unprivileged container", err)
			}
		}
		if len(starts) != 2 || starts["dependency"].After(starts["app"]) {
			t.Fatal("dependency restarted after dependent application activation")
		}
	}
	if err := runtime.ApplyQuadletGraph(ctx, staged, units); err != nil {
		t.Fatal("complete graph publication failed", err)
	}
	verify()
	// Reconciliation must retain the same order after the Core loses handles.
	runtime, err = targetsession.NewProjectRuntime(pool, scope)
	if err != nil {
		t.Fatal(err)
	}
	staged, err = runtime.RestoreProject(staged.Record(), source)
	if err != nil {
		t.Fatal("dependency graph commitment restoration failed", err)
	}
	if err := runtime.ApplyQuadletGraph(ctx, staged, units); err != nil {
		t.Fatal("dependency graph repair failed", err)
	}
	verify()
	if err := runtime.DestroyQuadletGraph(ctx, staged, units); err != nil {
		t.Fatal("owned dependency graph destruction failed", err)
	}
	services, err := runtime.ObserveProject(ctx, project)
	if err != nil || len(services) != 0 {
		t.Fatal("dependency graph survived authenticated destruction", err)
	}
	if f.inventory(t, ctx, pool, scope) == "" {
		t.Fatal("dependency graph destruction damaged foreign fixture")
	}
	t.Log("actual enrolled rootless Quadlet graph published both units before activation, started dependency before application, restored and repaired exact commitment, destroyed owned containers and preserved foreign fixture; full Application engine not qualified")
}
