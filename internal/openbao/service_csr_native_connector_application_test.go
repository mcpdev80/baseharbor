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
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// This exercises the same Application engine as the normal Core commands over
// production enrollment and an actual outbound Connector. It remains distinct
// from the still-required complete operator HTTP/Console release journey.
func (f *nativeConnectorFixture) applicationProject(t *testing.T, ctx context.Context, pool *targetsession.Pool, scope targetenrollment.Scope, issuer *ServiceIssuer) {
	t.Helper()
	manifest := application.New("remote-app-"+f.name[len(f.name)-16:], "dev", true, false, false)
	files, err := application.EnsureRuntime(ctx, issuer, application.Store{Root: filepath.Join(f.dir, "lifecycle-apps")}, manifest)
	if err != nil {
		t.Fatal("generate remote Application backend", err)
	}
	repository := filepath.Join(f.dir, "lifecycle-repository")
	if err := os.Mkdir(repository, 0700); err != nil {
		t.Fatal(err)
	}
	source := "services:\n  api:\n    image: " + f.image + "\n    user: '1000:1000'\n    read_only: true\n    command: ['sleep', '300']\n"
	if err := os.WriteFile(filepath.Join(repository, "compose.yaml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	workload, found, err := application.MaterializeWorkload(repository, manifest, files)
	if err != nil || !found {
		t.Fatal("materialize actual Application workload", err)
	}
	environment, err := application.RuntimeEnvironment(files)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := application.NewRemoteApplicationRuntime(pool, scope, files, manifest, &workload, environment)
	if err != nil {
		t.Fatal("compile consolidated Application publication", err)
	}
	state := filepath.Join(f.dir, "application-publication-state")
	if err := os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	var record targetsession.ProjectRecord
	if err := runtime.Publish(ctx, func(project targetsession.ProjectRecord) error {
		if err := runtime.SaveSnapshot(state, project); err != nil {
			return err
		}
		record = project
		return nil
	}); err != nil {
		t.Fatal("persist Application before activation", err)
	}
	// Emergency native removal establishes no approval. Successful teardown is
	// asserted below through the authenticated engine and native inventory.
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if f.engine == "podman" {
			root := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "containers", "systemd")
			for _, file := range record.Files {
				ext := filepath.Ext(file.Path)
				if ext != ".container" && ext != ".network" && ext != ".volume" {
					continue
				}
				base := strings.TrimSuffix(file.Path, ext)
				if ext != ".container" {
					base += "-" + strings.TrimPrefix(ext, ".")
				}
				_ = exec.CommandContext(cleanup, "systemctl", "--user", "stop", base+".service").Run()
				_ = os.Remove(filepath.Join(root, file.Path))
				_ = os.RemoveAll(filepath.Join(root, file.Path+".d"))
			}
			_ = exec.CommandContext(cleanup, "systemctl", "--user", "daemon-reload").Run()
		}
		for _, args := range [][]string{{"ps", "-aq", "--filter", "label=com.docker.compose.project=" + files.Project}, {"volume", "ls", "-q", "--filter", "label=com.docker.compose.project=" + files.Project}, {"network", "ls", "-q", "--filter", "label=com.docker.compose.project=" + files.Project}} {
			output, err := exec.CommandContext(cleanup, f.engine, args...).Output()
			if err != nil {
				t.Error("Application emergency inventory failed", err)
				continue
			}
			for _, id := range strings.Fields(string(output)) {
				remove := []string{"rm", "-f", id}
				if args[0] != "ps" {
					remove = []string{args[0], "rm", id}
				}
				if err := exec.CommandContext(cleanup, f.engine, remove...).Run(); err != nil {
					t.Error("Application emergency cleanup failed", err)
				}
			}
		}
	})
	if err := runtime.ApplyApplication(ctx, manifest, false); err != nil {
		t.Fatal("actual remote Application Apply", err)
	}
	observed, err := runtime.Observe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	providerID := ""
	for _, service := range observed {
		if service.Service == "postgres" {
			providerID = service.ID
		}
	}
	if providerID == "" {
		t.Fatal("Application provider has no owned native identity")
	}
	project, err := targetsession.NewProjectRuntime(pool, scope)
	if err != nil {
		t.Fatal(err)
	}
	if err := project.RemoveOwnedService(ctx, files.Project, "api", true); err != nil {
		t.Fatal("inject actual workload loss", err)
	}
	if runtime.VerifyApplication(ctx, manifest) == nil {
		t.Fatal("missing workload incorrectly reported ready")
	}
	// The changed checkout is deliberately unavailable to repair and teardown.
	if err := os.WriteFile(filepath.Join(repository, "compose.yaml"), []byte("services: {foreign: {image: forbidden}}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runtime, err = application.RestoreRemoteManagedSnapshot(pool, scope, state, record)
	if err != nil {
		t.Fatal("restore Application after source change", err)
	}
	if err := runtime.ApplyApplication(ctx, manifest, true); err != nil {
		t.Fatal("actual remote Application repair", err)
	}
	observed, err = runtime.Observe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, service := range observed {
		if service.Service == "postgres" && service.ID != providerID {
			t.Fatal("workload repair restarted healthy provider")
		}
	}
	if err := runtime.VerifyApplication(ctx, manifest); err != nil {
		t.Fatal("actual repaired Application status", err)
	}
	if err := runtime.DestroyApplication(ctx, manifest, true); err != nil {
		t.Fatal("actual Application owned reset and teardown", err)
	}
	if err := runtime.RemoveSnapshot(state); err != nil {
		t.Fatal("remove verified Application snapshot", err)
	}
	f.inventory(t, ctx, pool, scope)
	t.Log("actual enrolled remote Application engine published provider and prebuilt repository workload together, verified TLS SQL before workload activation, detected lost workload, restored immutable source for repair without restarting provider, verified status and owned reset/cleanup with foreign preservation; complete operator HTTP and Console journey not qualified")
}
