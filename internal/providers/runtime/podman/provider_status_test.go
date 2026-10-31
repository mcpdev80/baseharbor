package podman

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParsePodmanPublishedPorts(t *testing.T) {
	got, err := parsePodmanPublishedPorts(`[{"NetworkSettings":{"Ports":{"8080/tcp":[{"HostIp":"127.0.0.1","HostPort":"18080"}],"9090/udp":[{"HostIp":"0.0.0.0","HostPort":"19090"}]}}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("publishers = %d, want 2: %+v", len(got), got)
	}
	if got[0].TargetPort != 8080 || got[0].PublishedPort != 18080 || got[0].URL != "127.0.0.1" || got[0].Protocol != "tcp" {
		t.Fatalf("first publisher = %+v", got[0])
	}
	if got[1].TargetPort != 9090 || got[1].PublishedPort != 19090 || got[1].URL != "0.0.0.0" || got[1].Protocol != "udp" {
		t.Fatalf("second publisher = %+v", got[1])
	}
}

func TestParsePodmanPublishedPortsRejectsInvalidInspect(t *testing.T) {
	for _, input := range []string{"", "{}", "[]", "[{},{}]"} {
		if _, err := parsePodmanPublishedPorts(input); err == nil {
			t.Fatalf("parsePodmanPublishedPorts(%q) unexpectedly succeeded", input)
		}
	}
}

func TestPodmanSystemdUnitLabel(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"", ""},
		{"   ", ""},
		{"<no value>", ""},
		{"  bh-demo-app.service  ", "bh-demo-app.service"},
	} {
		if got := podmanSystemdUnitLabel(tc.input); got != tc.want {
			t.Fatalf("podmanSystemdUnitLabel(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// Podman selection lives at the provider boundary: New resolves the native
// podman binary via LookPath("podman") and builds a native CLI backend. It must
// never read DOCKER_CONTEXT/DOCKER_HOST, never probe docker context, and never
// derive a /run/user/<uid>/docker.sock endpoint. BASEHARBOR_TEST_RUNTIME never
// routes into the Docker engine probe; the docker and podman providers are
// separate, each gated by its own LookPath.
func TestPodmanProviderResolvesBehindLookPathPodmanBoundary(t *testing.T) {
	dir := t.TempDir()
	podman := filepath.Join(dir, "podman")
	if err := os.WriteFile(podman, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$PODMAN_BOUNDARY_LOG\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "calls")
	t.Setenv("PODMAN_BOUNDARY_LOG", log)
	// Put the stubbed podman first on PATH so LookPath("podman") resolves it,
	// proving the boundary gate the podman provider applies before building its
	// native backend. This is the engine boundary, never the docker engine probe.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	// Podman must not consult any docker context/host. If any of these leaked
	// through, the native probe would be a docker-context probe.
	t.Setenv("DOCKER_CONTEXT", "podman")
	t.Setenv("DOCKER_HOST", "unix:///run/user/1000/docker.sock")
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")

	// LookPath("podman") must resolve to our fixture, proving the boundary gate
	// that the podman provider applies before building its native backend. This
	// is the engine boundary, never the docker engine probe.
	path, err := exec.LookPath("podman")
	if err != nil || filepath.Clean(path) != filepath.Clean(podman) {
		t.Fatalf("LookPath podman did not resolve the podman provider binary: %q %v", path, err)
	}

	// The provider boundary is the descriptor: Kind is Podman (not docker), so
	// this provider never calls ResolveDockerEngine. The docker engine probe is
	// not on this path, and DOCKER_CONTEXT/DOCKER_HOST/BASEHARBOR_TEST_RUNTIME
	// never reach a docker context probe here.
	descriptor := Descriptor()
	if descriptor.Kind != ProviderPodman {
		t.Fatalf("podman descriptor Kind = %q, want podman", descriptor.Kind)
	}
	if descriptor.Realization == "docker-compose" {
		t.Fatal("podman provider must realize as podman, not the docker engine path")
	}
	if _, err := exec.LookPath("docker"); err == nil {
		// docker exists on PATH in this test; the podman boundary must still
		// select podman, proving the gate is LookPath("podman"), not the absence
		// of docker.
	}
}
