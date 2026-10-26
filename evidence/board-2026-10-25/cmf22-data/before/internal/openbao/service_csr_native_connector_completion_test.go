package openbao

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// Native fixtures test completion observation after production enrollment.
// They do not qualify source realization or the Application HTTP lifecycle.
func (f *nativeConnectorFixture) completedServices(t *testing.T, ctx context.Context, pool *targetsession.Pool, scope targetenrollment.Scope) {
	t.Helper()
	runtime, err := targetsession.NewProjectRuntime(pool, scope)
	if err != nil {
		t.Fatal(err)
	}
	project := f.name + "-init"
	for _, service := range []string{"done", "failed", "created", "running"} {
		script, operation := "exit 0", "run"
		if service == "failed" {
			script = "exit 17"
		}
		if service == "created" {
			operation = "create"
		}
		if service == "running" {
			script = "exec sleep 120"
		}
		args := []string{operation}
		if operation == "run" {
			args = append(args, "--detach")
		}
		args = append(args, "--name", project+"-"+service, "--user", "1000:1000", "--read-only",
			"--label", "com.docker.compose.project="+project, "--label", "com.docker.compose.service="+service,
			"--entrypoint", "/bin/sh", f.image, "-ec", script)
		output, err := exec.CommandContext(ctx, f.engine, args...).Output()
		if err != nil {
			t.Fatal("native completion fixture creation failed", service, err)
		}
		id := strings.TrimSpace(string(output))
		t.Cleanup(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			// Emergency cleanup never establishes qualification.
			_ = exec.CommandContext(cleanup, f.engine, "rm", "-f", id).Run()
		})
		if service == "done" || service == "failed" {
			wait, cancel := context.WithTimeout(ctx, 10*time.Second)
			output, err = exec.CommandContext(wait, f.engine, "wait", id).Output()
			cancel()
			want := "0"
			if service == "failed" {
				want = "17"
			}
			if err != nil || strings.TrimSpace(string(output)) != want {
				t.Fatal("native fixture did not reach its expected exit", service, err)
			}
		}
		err = runtime.VerifyCompletedService(ctx, project, service)
		if (err == nil) != (service == "done") {
			t.Fatal("native completion conflated stopped, running or failed service with success", service, err)
		}
	}
	observed, err := runtime.ObserveProject(ctx, project)
	if err != nil || len(observed) != 4 {
		t.Fatal("completion fixture inventory differs", err)
	}
	for _, service := range observed {
		if err := runtime.RemoveOwnedService(ctx, project, service.Service, true); err != nil {
			t.Fatal("authenticated completion fixture cleanup failed", err)
		}
	}
	observed, err = runtime.ObserveProject(ctx, project)
	if err != nil || len(observed) != 0 {
		t.Fatal("completion fixtures survived owned cleanup", err)
	}
	if err := runtime.VerifyCompletedService(ctx, project, "done"); err == nil {
		t.Fatal("removed service was accepted as completed")
	}
	if f.inventory(t, ctx, pool, scope) == "" {
		t.Fatal("completion cleanup damaged foreign fixture")
	}
	t.Log("authenticated native init completion distinguished successful exit 0, failure exit 17, never-started and running containers; removed container refused, owned cleanup verified and foreign fixture preserved; Quadlet init realization and full Application lifecycle not qualified")
}
