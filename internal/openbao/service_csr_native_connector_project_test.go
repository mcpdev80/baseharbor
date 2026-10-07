package openbao

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// This proves the Core's production project transport after real operator
// authorization and managed enrollment. It is not an Application engine proof.
func (f *nativeConnectorFixture) projectLifecycle(t *testing.T, ctx context.Context, pool *targetsession.Pool, scope targetenrollment.Scope) {
	t.Helper()
	runtime, err := targetsession.NewProjectRuntime(pool, scope)
	if err != nil {
		t.Fatal("managed project runtime did not bind enrolled node", err)
	}
	name := f.name + "-project"
	file := "compose.yaml"
	content := "name: " + name + "\nservices:\n  workload:\n    image: " + f.image + "\n    container_name: " + name + "\n    user: '1000:1000'\n    command: ['sleep','300']\n    labels:\n      baseharbor.enrollment-qualification: 'true'\n"
	if f.engine == "podman" {
		file = name + ".container"
		content = "[Unit]\nDescription=BaseHarbor managed project qualification\n[Container]\nImage=" + f.image + "\nContainerName=" + name + "\nUser=1000:1000\nExec=sleep 300\nLabel=baseharbor.enrollment-qualification=true\nLabel=com.docker.compose.project=" + name + "\nLabel=com.docker.compose.service=workload\n[Service]\nTimeoutStartSec=45\n[Install]\nWantedBy=default.target\n"
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if f.engine == "podman" {
			_ = exec.CommandContext(cleanup, "systemctl", "--user", "stop", name+".service").Run()
			root := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "containers", "systemd")
			_ = os.Remove(filepath.Join(root, file))
			_ = os.RemoveAll(filepath.Join(root, file+".d"))
			_ = exec.CommandContext(cleanup, "systemctl", "--user", "daemon-reload").Run()
		}
		_ = exec.CommandContext(cleanup, f.engine, "rm", "-f", name).Run()
		if f.engine == "docker" {
			_ = exec.CommandContext(cleanup, f.engine, "network", "rm", name+"_default").Run()
		}
		output, err := exec.CommandContext(cleanup, f.engine, "ps", "-aq", "--filter", "name=^"+name+"$").Output()
		if err != nil || strings.TrimSpace(string(output)) != "" {
			t.Error("owned managed project remained after cleanup", err)
		}
		// Emergency cleanup only prevents leaking a failed fixture. Success is
		// asserted through authenticated Core inventory before this callback.
	})
	staged, err := runtime.Stage(ctx, name, []targetsession.ProjectFile{{Path: file, Data: []byte(content)}})
	if err != nil {
		t.Fatal("managed project staging failed", err)
	}
	apply := func(repair bool) {
		t.Helper()
		var err error
		if f.engine == "docker" {
			err = runtime.ApplyCompose(ctx, staged, []string{file}, "", repair)
		} else {
			err = runtime.ApplyQuadlet(ctx, staged, file)
		}
		if err != nil {
			t.Fatal("managed project realization failed", err)
		}
		f.projectObserved(t, ctx, pool, scope, name, true)
	}
	apply(false)
	services, err := runtime.ObserveProject(ctx, name)
	if err != nil || len(services) != 1 || services[0].Service != "workload" || !services[0].Running {
		t.Fatal("Core project observation did not verify native ownership", err)
	}
	output, err := runtime.ExecService(ctx, name, "workload", "id", "-u")
	if err != nil || strings.TrimSpace(output) != "1000" {
		t.Fatal("Core project service probe did not execute in actual owned container", err)
	}
	if _, err := runtime.ExecService(ctx, name, "foreign", "true"); err == nil {
		t.Fatal("undeclared project service reached execution")
	}
	// Lose all in-memory staged handles. The protected Core receipt and exact
	// original source must reconcile the same immutable publication on retry.
	recordPath := filepath.Join(f.dir, "owned-project-receipt.json")
	data, err := json.Marshal(staged.Record())
	if err != nil || os.WriteFile(recordPath, data, 0600) != nil {
		t.Fatal("Core project receipt could not be persisted", err)
	}
	data, err = os.ReadFile(recordPath)
	if err != nil {
		t.Fatal("Core project receipt could not be loaded", err)
	}
	var record targetsession.ProjectRecord
	if json.Unmarshal(data, &record) != nil {
		t.Fatal("persisted Core project receipt is invalid")
	}
	runtime, err = targetsession.NewProjectRuntime(pool, scope)
	if err != nil {
		t.Fatal("Core project runtime did not rebind after handle loss", err)
	}
	staged, err = runtime.RestoreProject(record, []targetsession.ProjectFile{{Path: file, Data: []byte(content)}})
	if err != nil {
		t.Fatal("Core project retry did not restore exact immutable publication", err)
	}
	apply(true)
	if f.engine == "docker" {
		err = runtime.DestroyCompose(ctx, staged, []string{file}, "")
	} else {
		err = runtime.DestroyQuadlet(ctx, staged, file)
	}
	if err != nil {
		t.Fatal("managed project destroy failed", err)
	}
	f.projectObserved(t, ctx, pool, scope, name, false)
	services, err = runtime.ObserveProject(ctx, name)
	if err != nil || len(services) != 0 {
		t.Fatal("destroyed project still observable through Core adapter", err)
	}
	if f.inventory(t, ctx, pool, scope) == "" {
		t.Fatal("project destroy damaged foreign fixture")
	}
	t.Log("managed Core project staging, apply, observed running state, repair and destroy preserved foreign fixture")
	if f.engine == "podman" {
		f.quadletDependencyGraph(t, ctx, pool, scope)
		f.quadletCompletion(t, ctx, pool, scope)
	}
}

func (f *nativeConnectorFixture) projectObserved(t *testing.T, ctx context.Context, pool *targetsession.Pool, scope targetenrollment.Scope, name string, want bool) {
	t.Helper()
	response, err := f.dispatch(ctx, pool, scope, "runtime.resource.list", struct{}{})
	if err != nil || !response.Success {
		t.Fatal("managed project inventory unavailable", err)
	}
	var resources []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		State string `json:"state"`
	}
	if json.Unmarshal(response.Result, &resources) != nil {
		t.Fatal("invalid managed project inventory")
	}
	found := false
	for _, resource := range resources {
		if strings.TrimPrefix(resource.Name, "/") != name {
			continue
		}
		if resource.ID == "" || resource.State != "running" {
			t.Fatal("managed project has not converged")
		}
		found = true
	}
	if found != want {
		t.Fatal("managed project runtime presence differs", want, found)
	}
}
