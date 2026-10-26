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

func (f *nativeConnectorFixture) composePhases(t *testing.T, ctx context.Context, pool *targetsession.Pool, scope targetenrollment.Scope) {
	t.Helper()
	runtime, err := targetsession.NewProjectRuntime(pool, scope)
	if err != nil {
		t.Fatal(err)
	}
	name := f.name + "-phases"
	content := "name: " + name + "\nservices:\n  provider:\n    image: " + f.image + "\n    user: '1000:1000'\n    command: [sleep, '300']\n    labels: {baseharbor.enrollment-qualification: 'true'}\n  workload:\n    image: " + f.image + "\n    user: '1000:1000'\n    command: [sleep, '300']\n    depends_on: [provider]\n    labels: {baseharbor.enrollment-qualification: 'true'}\n"
	files := []targetsession.ProjectFile{{Path: "compose.yaml", Data: []byte(content)}}
	project, err := runtime.Stage(ctx, name, files)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		out, err := exec.CommandContext(cleanup, "docker", "ps", "-aq", "--filter", "label=com.docker.compose.project="+name).Output()
		if err != nil {
			t.Error("phase cleanup inventory failed")
			return
		}
		for _, id := range strings.Fields(string(out)) {
			_ = exec.CommandContext(cleanup, "docker", "rm", "-f", id).Run()
		}
		_ = exec.CommandContext(cleanup, "docker", "network", "rm", name+"_default").Run()
	})
	// Restore the immutable commitment before executing either phase.
	project, err = runtime.RestoreProject(project.Record(), files)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ApplyComposeSelected(ctx, project, []string{"compose.yaml"}, "", []string{"provider"}, false); err != nil {
		t.Fatal("provider phase failed", err)
	}
	before, err := runtime.ObserveProject(ctx, name)
	if err != nil || len(before) != 1 || before[0].Service != "provider" || !before[0].Running {
		t.Fatal("provider phase activated unselected workload", err)
	}
	for _, repair := range []bool{false, true} {
		if err := runtime.ApplyComposeSelected(ctx, project, []string{"compose.yaml"}, "", []string{"workload"}, repair); err != nil {
			t.Fatal("workload phase failed", err)
		}
		observed, err := runtime.ObserveProject(ctx, name)
		if err != nil || len(observed) != 2 {
			t.Fatal("phase observation failed", err)
		}
		for _, service := range observed {
			if !service.Running || (service.Service == "provider" && service.ID != before[0].ID) {
				t.Fatal("workload phase restarted its prerequisite")
			}
		}
	}
	if err := runtime.DestroyCompose(ctx, project, []string{"compose.yaml"}, ""); err != nil {
		t.Fatal("phase graph destroy failed", err)
	}
	observed, err := runtime.ObserveProject(ctx, name)
	if err != nil || len(observed) != 0 || f.inventory(t, ctx, pool, scope) == "" {
		t.Fatal("phase graph cleanup or foreign preservation failed", err)
	}
	t.Log("actual enrolled Docker immutable project activated provider before workload and repaired workload without restarting provider; owned cleanup and foreign preservation passed; full Application engine not qualified")
}
