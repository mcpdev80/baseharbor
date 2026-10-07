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

	"github.com/mcpdev80/baseharbor/internal/runtime/remoteprojection"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// Records a native observation after real authenticated completion and before
// Core dispatches dependent activation. No runtime responses are substituted.
type nativeInitObservation struct {
	*targetsession.Pool
	t     *testing.T
	start map[string]string
}

func (n *nativeInitObservation) Dispatch(ctx context.Context, scope targetenrollment.Scope, request targetsession.Request) (targetsession.Response, error) {
	response, err := n.Pool.Dispatch(ctx, scope, request)
	if err == nil && response.Success && request.Operation == "runtime.quadlet.verify-completion" {
		var payload struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil {
			n.t.Fatal("invalid completion selector")
		}
		unit := strings.TrimSuffix(payload.Name, ".container") + ".service"
		if _, exists := n.start[unit]; !exists {
			n.start[unit] = nativeInitStart(n.t, ctx, unit)
		}
	}
	return response, err
}

func nativeInitStart(t *testing.T, ctx context.Context, unit string) string {
	t.Helper()
	data, err := exec.CommandContext(ctx, "systemctl", "--user", "show", "--property=ExecMainStartTimestampMonotonic", "--value", unit).Output()
	value := strings.TrimSpace(string(data))
	if err != nil || value == "" || value == "0" {
		t.Fatal("native init start observation unavailable", err)
	}
	return value
}

func (f *nativeConnectorFixture) quadletInitGraph(t *testing.T, ctx context.Context, pool *targetsession.Pool, scope targetenrollment.Scope) {
	t.Helper()
	for _, scenario := range []string{"success", "failed"} {
		t.Run("init-graph-"+scenario, func(t *testing.T) {
			name := f.name + "-initgraph-" + scenario
			exit := "0"
			if scenario == "failed" {
				exit = "17"
			}
			compose := filepath.Join(t.TempDir(), "compose.yaml")
			data := "services:\n  app:\n    image: " + f.image + "\n    user: '1000:1000'\n    read_only: true\n    command: [sleep, '300']\n    depends_on:\n      init:\n        condition: service_completed_successfully\n  init:\n    image: " + f.image + "\n    user: '1000:1000'\n    read_only: true\n    command: [sh, -ec, 'sleep 1; exit " + exit + "']\n"
			if err := os.WriteFile(compose, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			graph, err := remoteprojection.ProjectRemoteQuadletInitGraph(compose, "", name, nil)
			if err != nil {
				t.Fatal("generated init graph projection failed", err)
			}
			var source []targetsession.ProjectFile
			for file, content := range graph.Files {
				source = append(source, targetsession.ProjectFile{Path: file, Data: []byte(content)})
			}
			transport := &nativeInitObservation{Pool: pool, t: t, start: map[string]string{}}
			runtime, err := targetsession.NewProjectRuntime(transport, scope)
			if err != nil {
				t.Fatal(err)
			}
			staged, err := runtime.Stage(ctx, name, source)
			if err != nil {
				t.Fatal("generated init graph staging failed", err)
			}
			t.Cleanup(func() {
				cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
				defer stop()
				// Emergency fixture cleanup is separate from observed owned destroy.
				root := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "containers", "systemd")
				for _, file := range graph.Units {
					if strings.HasSuffix(file, ".container") {
						unit := strings.TrimSuffix(file, ".container") + ".service"
						_ = exec.CommandContext(cleanup, "systemctl", "--user", "stop", unit).Run()
					}
					_ = os.Remove(filepath.Join(root, file))
					_ = os.RemoveAll(filepath.Join(root, file+".d"))
				}
				for _, service := range []string{"app", "init"} {
					_ = exec.CommandContext(cleanup, "podman", "rm", "-f", name+"-"+service).Run()
				}
				_ = exec.CommandContext(cleanup, "podman", "network", "rm", name+"_default").Run()
				_ = exec.CommandContext(cleanup, "systemctl", "--user", "daemon-reload").Run()
			})
			apply, cancel := context.WithTimeout(ctx, 10*time.Second)
			err = runtime.ApplyQuadletInitGraph(apply, staged, graph.Units, graph.InitUnits)
			cancel()
			if (err == nil) != (scenario == "success") {
				t.Fatal("generated init dependency qualification differs", err)
			}
			services, err := runtime.ObserveProject(ctx, name)
			if err != nil {
				t.Fatal("generated init graph observation failed", err)
			}
			appRunning := false
			for _, service := range services {
				if service.Service == "app" && service.Running {
					appRunning = true
				}
			}
			if appRunning != (scenario == "success") {
				t.Fatal("failed init released dependent or successful init did not release it")
			}
			if scenario == "success" {
				if len(transport.start) != 1 {
					t.Fatal("dependent started without exact init observation")
				}
				for unit, start := range transport.start {
					if nativeInitStart(t, ctx, unit) != start {
						t.Fatal("dependent activation implicitly reran completed init")
					}
				}
				output, err := runtime.ExecService(ctx, name, "app", "id", "-u")
				if err != nil || strings.TrimSpace(output) != "1000" {
					t.Fatal("dependent did not run unprivileged", err)
				}
			}
			if err := runtime.DestroyQuadletGraph(ctx, staged, graph.Units); err != nil {
				t.Fatal("generated init graph destroy failed", err)
			}
			services, err = runtime.ObserveProject(ctx, name)
			if err != nil || len(services) != 0 {
				t.Fatal("generated init graph retained owned containers", err)
			}
		})
	}
	if f.inventory(t, ctx, pool, scope) == "" {
		t.Fatal("generated init graph damaged foreign fixture")
	}
	if !t.Failed() {
		t.Log("actual enrolled rootless generated init graph verified exact completion before dependent activation, refused failed init without activating application, did not rerun successful init, ran application as UID 1000 and destroyed owned graph with foreign preservation; full Application engine not qualified")
	}
}
