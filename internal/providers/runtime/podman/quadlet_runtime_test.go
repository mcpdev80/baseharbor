package podman

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestQuadletProjectResourceUnitsOrdersPrerequisites(t *testing.T) {
	project := QuadletProject{Files: map[string]string{
		"demo-z.network":     "",
		"demo-a.network":     "",
		"demo-data.volume":   "",
		"demo-api.build":     "",
		"demo-api.container": "",
	}}
	networks, volumes, builds := quadletProjectResourceUnits(project)
	if got, want := len(networks), 2; got != want {
		t.Fatalf("networks = %d, want %d", got, want)
	}
	if networks[0] != "demo-a-network.service" || networks[1] != "demo-z-network.service" {
		t.Fatalf("network units not sorted: %#v", networks)
	}
	if len(volumes) != 1 || volumes[0] != "demo-data-volume.service" {
		t.Fatalf("volume units = %#v", volumes)
	}
	if len(builds) != 1 || builds[0] != "demo-api-build.service" {
		t.Fatalf("build units = %#v", builds)
	}
}

func TestQuadletDirectiveValueReadsResourceNames(t *testing.T) {
	content := "[Network]\nNetworkName=baseharbor-demo_default\nLabel=key=value\n"
	if got := quadletDirectiveValue(content, "NetworkName"); got != "baseharbor-demo_default" {
		t.Fatalf("NetworkName = %q", got)
	}
	if got := quadletDirectiveValue(content, "VolumeName"); got != "" {
		t.Fatalf("unexpected VolumeName = %q", got)
	}
}

func TestQuadletProjectInstalledUnchanged(t *testing.T) {
	dir := t.TempDir()
	project := QuadletProject{
		Project: "baseharbor-demo",
		Files: map[string]string{
			"baseharbor-demo-api.container":   "[Container]\nImage=example\n",
			"baseharbor-demo-default.network": "[Network]\nNetworkName=baseharbor-demo_default\n",
		},
	}
	for name, content := range project.Files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	unchanged, err := quadletProjectInstalledUnchanged(dir, project)
	if err != nil {
		t.Fatal(err)
	}
	if !unchanged {
		t.Fatal("expected installed project to be unchanged")
	}

	if err := os.WriteFile(filepath.Join(dir, "baseharbor-demo-api.container"), []byte("[Container]\nImage=changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unchanged, err = quadletProjectInstalledUnchanged(dir, project)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged {
		t.Fatal("expected modified installed project to require reconciliation")
	}
}

func TestQuadletFileForUnitResolvesManagedResources(t *testing.T) {
	project := QuadletProject{Files: map[string]string{
		"demo-app.container":    "",
		"demo-internal.network": "[Network]\nNetworkName=demo_internal\n",
		"demo-data.volume":      "[Volume]\nVolumeName=demo_data\n",
	}}
	if got := quadletFileForUnit(project, "demo-internal-network.service", "network"); got != "demo-internal.network" {
		t.Fatalf("network file = %q", got)
	}
	if got := quadletFileForUnit(project, "demo-data-volume.service", "volume"); got != "demo-data.volume" {
		t.Fatalf("volume file = %q", got)
	}
	if got := quadletFileForUnit(project, "demo-app.service", "network"); got != "" {
		t.Fatalf("unexpected resource file = %q", got)
	}
}

func TestQuadletProjectBuildUnitsSelectsOnlyChangedServices(t *testing.T) {
	project := QuadletProject{
		Project: "baseharbor-demo",
		Files: map[string]string{
			"baseharbor-demo-api.build":        "",
			"baseharbor-demo-api.container":    "",
			"baseharbor-demo-worker.build":     "",
			"baseharbor-demo-worker.container": "",
		},
	}
	got := quadletProjectBuildUnits(project, []string{"worker"})
	if len(got) != 1 || got[0] != "baseharbor-demo-worker-build.service" {
		t.Fatalf("selected build units = %#v, want worker only", got)
	}

	all := quadletProjectBuildUnits(project, nil)
	if len(all) != 2 ||
		all[0] != "baseharbor-demo-api-build.service" ||
		all[1] != "baseharbor-demo-worker-build.service" {
		t.Fatalf("all build units = %#v", all)
	}
}

func TestQuadletUserUnitDirIgnoresApplicationXDGConfigIsolation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "isolated-app-config"))

	unitDir, err := quadletUserUnitDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "containers", "systemd")
	if unitDir != want {
		t.Fatalf("unit dir = %q, want systemd-visible path %q", unitDir, want)
	}
}

func TestQuadletChangedServiceUnitsDetectsDefinitionAndEnvironmentChanges(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "isolated-app-config"))
	unitDir, err := quadletUserUnitDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(unitDir, 0o700); err != nil {
		t.Fatal(err)
	}

	project := QuadletProject{
		Project: "baseharbor-demo",
		Files: map[string]string{
			"baseharbor-demo-api.container":    "[Container]\nImage=example:new\nEnvironmentFile=baseharbor-demo-api.env\n",
			"baseharbor-demo-api.env":          "MODE=new\n",
			"baseharbor-demo-worker.container": "[Container]\nImage=worker\n",
		},
	}
	installed := map[string]string{
		"baseharbor-demo-api.container":    "[Container]\nImage=example:old\nEnvironmentFile=baseharbor-demo-api.env\n",
		"baseharbor-demo-api.env":          "MODE=old\n",
		"baseharbor-demo-worker.container": "[Container]\nImage=worker\n",
	}
	for name, content := range installed {
		if err := os.WriteFile(filepath.Join(unitDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	units, err := quadletChangedServiceUnits(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 || units[0] != "baseharbor-demo-api.service" {
		t.Fatalf("changed units = %#v, want api service only", units)
	}
}

func TestQuadletRemoveRuntimeResourceArgsDoesNotForceNetworks(t *testing.T) {
	got := quadletRemoveRuntimeResourceArgs("network", "shared-net")
	want := []string{"network", "rm", "shared-net"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("network remove args = %#v, want %#v", got, want)
	}
}

func TestQuadletRemoveRuntimeResourceArgsStillForcesVolumes(t *testing.T) {
	got := quadletRemoveRuntimeResourceArgs("volume", "data")
	want := []string{"volume", "rm", "-f", "data"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("volume remove args = %#v, want %#v", got, want)
	}
}

func TestQuadletNetworkResourceInUseRecognizesPodmanErrors(t *testing.T) {
	for _, message := range []string{
		`Error: "demo" has associated containers with it. Use -f to forcibly delete containers and pods: network is being used`,
		"network demo has active endpoints",
	} {
		if !quadletNetworkResourceInUse(message) {
			t.Fatalf("expected in-use network error to be recognized: %q", message)
		}
	}
	if quadletNetworkResourceInUse("network not found") {
		t.Fatal("unrelated network error must not be ignored")
	}
}

func TestQuadletRemoveRuntimeResourceArgsForcesContainers(t *testing.T) {
	got := quadletRemoveRuntimeResourceArgs("container", "app")
	want := []string{"container", "rm", "-f", "app"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}
