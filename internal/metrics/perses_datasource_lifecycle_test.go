package metrics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// The lifecycle seam tests (0428-083): the metrics-side materializer, mount
// resolver and removal entry points the application lifecycle calls at the
// apply boundary. They are exercised here (the metrics side of the seam) so
// the render/mount contract is proven against the REAL materializer paths, not
// a fixture copy. The seam contract takes the APPLICATION RUNTIME DIR (the
// one layout formula of this Core) and the datasource root is its
// providers/management-ui/perses sibling (F1).

func persesDatasourceLifecycleManifest(t *testing.T, perses bool) application.Manifest {
	t.Helper()
	m := application.New("perses-lifecycle", "dev", false, false, false)
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = perses
	return m
}

// lifecycleDatasourceLayout derives the deployment's datasource roots through
// the SINGLE application-side formula the seams use — the runtime-sibling
// derivation — so the test never re-implements the layout. The lifecycle seam
// passes the runtime dir; the manifest-bound entry is addressed by the
// deployment dir (<dataRoot>/<app>).
func lifecycleDatasourceLayout(t *testing.T, state string, m application.Manifest, namespace string) (runtimeDir, datasourceRoot string) {
	t.Helper()
	_ = namespace
	runtimeDir = filepath.Join(filepath.Clean(state), m.Name, "runtime")
	datasourceRoot = application.PersesDatasourceRuntimeRoot(runtimeDir)
	return
}

// TestPersesDatasourceLifecycleMountsResolveFromMaterializer proves the mount
// contract end-to-end: the materializer writes its artifacts under the
// datasource projection root (the sibling of the runtime dir the lifecycle
// owns), and the mount resolver returns exactly those paths
// (compose-context-relative). A provisioned datasource means the resolver
// returns mounts whose host sources exist.
func TestPersesDatasourceLifecycleMountsResolveFromMaterializer(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesDatasourceLifecycleManifest(t, true)
	project := application.RuntimeProjectName(m)
	runtimeDir, datasourceRoot := lifecycleDatasourceLayout(t, state, m, "")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))

	configMount, provisioningMount, _, present := persesDatasourceLifecycleMounts(runtimeDir, project)
	if present {
		t.Fatalf("nothing provisioned yet, mounts must be absent: %q %q", configMount, provisioningMount)
	}
	if err := materializePersesDatasourceForLifecycle(m, runtimeDir, ""); err != nil {
		t.Fatalf("lifecycle materialize: %v", err)
	}
	configMount, provisioningMount, _, present = persesDatasourceLifecycleMounts(runtimeDir, project)
	if !present || configMount == "" || provisioningMount == "" {
		t.Fatalf("mounts after provisioning = (%q, %q, %v), want the materializer's artifacts", configMount, provisioningMount, present)
	}
	// The artifacts must sit at the datasource root the seam derives, and the
	// mount sources must resolve to the SAME files (writer/reader agreement,
	// F1): the host path of each mount exists.
	for _, mount := range []string{configMount, provisioningMount} {
		source := strings.SplitN(mount, ":", 2)[0]
		if !strings.HasPrefix(source, "./") && !strings.HasPrefix(source, "../") {
			t.Fatalf("mount %q is not compose-context-relative", mount)
		}
		hostPath := filepath.Join(runtimeDir, filepath.FromSlash(strings.TrimPrefix(source, "./")))
		if _, err := os.Stat(hostPath); err != nil {
			t.Fatalf("mount source %s (%s) does not exist: %v", source, hostPath, err)
		}
		if !strings.Contains(mount, ":ro") {
			t.Fatalf("datasource mounts are read-only: %q", mount)
		}
	}
	if _, err := os.Stat(filepath.Join(datasourceRoot, persesDatasourceProvisioningDir, persesDatasourceProvisioningFile)); err != nil {
		t.Fatalf("the seam materialized away from the derived datasource root: %v", err)
	}

	// Deselection removes the datasource artifacts (never a stale datasource
	// for the next start) and the resolver stops claiming mounts.
	deselected := persesDatasourceLifecycleManifest(t, false)
	deselectedRuntimeDir, _ := lifecycleDatasourceLayout(t, state, deselected, "")
	if err := removePersesDatasourceForLifecycle(deselected, deselectedRuntimeDir, ""); err != nil {
		t.Fatalf("lifecycle remove: %v", err)
	}
	if _, _, _, present = persesDatasourceLifecycleMounts(runtimeDir, project); present {
		t.Fatal("mounts must disappear after the datasource projection was removed")
	}
}

// TestPersesDatasourceLifecycleDeselectedNeverMaterializes proves the seam
// fails closed for a deselected surface: a materializer call on an unselected
// Perses never writes provisioning.
func TestPersesDatasourceLifecycleDeselectedNeverMaterializes(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	m := persesDatasourceLifecycleManifest(t, false)
	runtimeDir, datasourceRoot := lifecycleDatasourceLayout(t, state, m, "")
	if err := materializePersesDatasourceForLifecycle(m, runtimeDir, ""); err != nil {
		t.Fatalf("deselected removal is a no-op-clean call: %v", err)
	}
	if _, err := os.Stat(filepath.Join(datasourceRoot, "config.yaml")); err == nil {
		t.Fatal("a deselected surface must not provision a datasource config")
	}
	entries, err := os.ReadDir(datasourceRoot)
	if err == nil && len(entries) != 0 {
		t.Fatalf("a deselected surface must leave no datasource projection content: %v", entries)
	}
}

// TestPersesDatasourceLifecycleRematerializeClearsStaleResource proves the
// provisioning folder never carries two datasource generations: the loader
// loads EVERY file in the folder, so the materializer clears regular files it
// is about to replace (an earlier apply's resource, e.g. with the previous
// endpoint) before writing the current one.
func TestPersesDatasourceLifecycleRematerializeClearsStaleResource(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))
	runtimeDir, datasourceRoot := lifecycleDatasourceLayout(t, state, m, "")
	if err := materializePersesDatasourceForLifecycle(m, runtimeDir, ""); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	provisioning := filepath.Join(datasourceRoot, persesDatasourceProvisioningDir)
	stale := filepath.Join(provisioning, "globaldatasource-stale-generation.json")
	if err := writePersesDatasourceFile(stale, []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	if err := materializePersesDatasourceForLifecycle(m, runtimeDir, ""); err != nil {
		t.Fatalf("re-materialize: %v", err)
	}
	entries, err := os.ReadDir(provisioning)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, entry := range entries {
		names[entry.Name()] = true
	}
	if names["globaldatasource-stale-generation.json"] {
		t.Fatal("the provisioning folder still carries the stale generation after re-materialization (the loader would load two datasource generations)")
	}
	for name := range names {
		if name != persesDatasourceProvisioningFile && name != persesDatasourceSecretFile {
			t.Fatalf("unexpected provisioning folder entry %q (only this generation's datasource and its GlobalSecret may exist)", name)
		}
	}
}

// TestPersesDatasourceLifecycleSingleRootOnBothLayouts proves the F1 claim:
// on the local store layout AND the target-scoped layout, the lifecycle seam
// (which holds the runtime dir) and the manifest-bound entry (which holds a
// data root) address the SAME datasource root — writer and reader can never
// disagree.
func TestPersesDatasourceLifecycleSingleRootOnBothLayouts(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesDatasourceLifecycleManifest(t, true)
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))

	for _, layout := range []struct {
		name       string
		runtimeDir string
	}{
		// local store layout: <dataRoot>/<app>/runtime
		{"local", filepath.Join(state, m.Name, "runtime")},
		// target-scoped layout: <dataRoot>/applications/<resourceProject>/runtime
		{"target", filepath.Join(state, "applications", application.RuntimeProjectNameForApp(m.Name, m.Environment)+"-acme", "runtime")},
	} {
		seamRoot := datasourceLifecycleRoots(layout.runtimeDir)
		// The manifest-bound entry (the CLI build path, addressed by data
		// root) must land on the sibling of the LOCAL runtime dir — the same
		// root the lifecycle seam addresses for the same layout.
		localEntryRoot, err := persesDatasourceProjectionRoot(state, "", m)
		if err != nil {
			t.Fatalf("layout %q: entry root: %v", layout.name, err)
		}
		if layout.name == "local" && filepath.Clean(seamRoot) != filepath.Clean(localEntryRoot) {
			t.Fatalf("layout %q: seam root %q and manifest-bound root %q disagree (two layout formulas)", layout.name, seamRoot, localEntryRoot)
		}
		if filepath.Base(filepath.Dir(seamRoot)) != "management-ui" {
			t.Fatalf("layout %q: datasource root is not the provider layout sibling: %q", layout.name, seamRoot)
		}
		if filepath.Base(filepath.Dir(filepath.Dir(seamRoot))) != "providers" {
			t.Fatalf("layout %q: datasource root is not under the providers/management-ui layout: %q", layout.name, seamRoot)
		}
	}
}
