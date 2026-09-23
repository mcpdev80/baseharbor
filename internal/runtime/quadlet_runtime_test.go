package runtime

import "testing"

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
