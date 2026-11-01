package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerEngineSelectionPrecedenceAndNoFallback(t *testing.T) {
	originalUserNamespaceProbe := rootlessCapable
	originalSocketProbe := rootlessSocketExists
	t.Cleanup(func() {
		rootlessCapable = originalUserNamespaceProbe
		rootlessSocketExists = originalSocketProbe
	})
	for _, tc := range []struct {
		name             string
		selection        DockerEngineSelection
		env              map[string]string
		active, endpoint string
		unavailable      bool
		invalid          bool
		socketAbsent     bool
		rootlessAt       string
		wantOrigin       string
		hostRootful      bool
		wantMode         string
	}{
		{name: "rootless-socket-host-default", active: "default", endpoint: "unix:///run/user/1000/docker.sock", wantOrigin: "active-default"},
		{name: "system-daemon-host-default", active: "default", endpoint: "unix:///var/run/docker.sock", socketAbsent: true, hostRootful: true, wantOrigin: "active-default", wantMode: "rootful"},
		{name: "named-rootless-context", active: "rootless", endpoint: "unix:///run/user/1000/docker.sock", socketAbsent: true, wantOrigin: "active-context"},
		{name: "named-system-context", selection: DockerEngineSelection{Mode: "rootful"}, active: "system", endpoint: "unix:///var/run/docker.sock", socketAbsent: true, wantOrigin: "active-context", wantMode: "rootful"},
		{name: "rootless-on-system-socket-context", active: "rootless", endpoint: "unix:///var/run/docker.sock", rootlessAt: "unix:///var/run/docker.sock", socketAbsent: true, wantOrigin: "active-context"},
		{name: "rootless-daemon-under-system-socket", active: "default", endpoint: "unix:///var/run/docker.sock", rootlessAt: "unix:///var/run/docker.sock", socketAbsent: true, hostRootful: true, wantOrigin: "active-default"},
		{name: "no-daemon-fails-closed", active: "default", endpoint: "unix:///var/run/docker.sock", socketAbsent: true, hostRootful: true, unavailable: true, invalid: true},
		{name: "rootless-context-requires-rootless-daemon", selection: DockerEngineSelection{Context: "system", Mode: "rootless"}, endpoint: "unix:///var/run/docker.sock", hostRootful: true, invalid: true},
		{name: "rootful-context-requires-rootful-daemon", selection: DockerEngineSelection{Context: "rootless", Mode: "rootful"}, endpoint: "unix:///run/user/1000/docker.sock", invalid: true},
		{name: "target-endpoint-wins", selection: DockerEngineSelection{Endpoint: "unix:///selected.sock", Mode: "rootful"}, env: map[string]string{"DOCKER_CONTEXT": "other", "DOCKER_HOST": "unix:///wrong.sock"}, endpoint: "unix:///selected.sock", socketAbsent: true, hostRootful: true, wantOrigin: "target-endpoint", wantMode: "rootful"},
		{name: "target-context-wins", selection: DockerEngineSelection{Context: "chosen", Mode: "rootful"}, env: map[string]string{"DOCKER_CONTEXT": "other"}, endpoint: "unix:///selected.sock", socketAbsent: true, hostRootful: true, wantOrigin: "target-context", wantMode: "rootful"},
		{name: "environment-context-wins", selection: DockerEngineSelection{Mode: "rootful"}, env: map[string]string{"DOCKER_CONTEXT": "chosen", "DOCKER_HOST": "unix:///wrong.sock"}, endpoint: "unix:///selected.sock", socketAbsent: true, hostRootful: true, wantOrigin: "environment-context", wantMode: "rootful"},
		{name: "environment-endpoint", selection: DockerEngineSelection{Mode: "rootful"}, env: map[string]string{"DOCKER_HOST": "unix:///selected.sock"}, endpoint: "unix:///selected.sock", socketAbsent: true, hostRootful: true, wantOrigin: "environment-endpoint", wantMode: "rootful"},
		{name: "rootless-capable-host-dead-socket-still-fails-closed", active: "default", endpoint: "unix:///run/user/1000/docker.sock", socketAbsent: true, unavailable: true, invalid: true},
		{name: "unreachable-never-falls-back", active: "rootless", endpoint: "unix:///run/user/1000/docker.sock", unavailable: true, invalid: true},
		{name: "target-rootful-with-rootless-socket", selection: DockerEngineSelection{Context: "system", Mode: "rootful"}, endpoint: "unix:///var/run/docker.sock", hostRootful: true, wantOrigin: "target-context", wantMode: "rootful"},
		{name: "explicit-rootless-unreachable-never-falls-back", selection: DockerEngineSelection{Context: "rootless", Mode: "rootless"}, endpoint: "unix:///run/user/1000/docker.sock", unavailable: true, invalid: true},
		{name: "explicit-rootful-maintenance", selection: DockerEngineSelection{Endpoint: "unix:///var/run/docker.sock", Mode: "rootful"}, endpoint: "unix:///var/run/docker.sock", hostRootful: true, wantOrigin: "target-endpoint", wantMode: "rootful"},
		{name: "conflicting-target-selection", selection: DockerEngineSelection{Endpoint: "unix:///a.sock", Context: "chosen"}, invalid: true},
		{name: "plaintext-remote-denied", selection: DockerEngineSelection{Endpoint: "tcp://remote:2375"}, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			socketPresent := map[string]bool{
				rootlessSocketPath(1000): !tc.socketAbsent,
				systemSocketPath:         true,
			}
			if tc.hostRootful {
				socketPresent[rootlessSocketPath(1000)] = false
			}
			rootlessSocketExists = func(path string) bool { return socketPresent[path] }
			rootlessCapable = func() bool { return !tc.hostRootful }
			var probes []string
			run := func(_ context.Context, _ string, args []string, env []string) ([]byte, error) {
				if args[0] == "context" {
					if args[1] == "show" {
						return []byte(tc.active), nil
					}
					return []byte(`"` + tc.endpoint + `"`), nil
				}
				if len(args) < 4 || args[0] != "--host" || args[2] != "info" {
					t.Fatalf("unexpected command: %v", args)
				}
				probes = append(probes, args[1])
				for _, item := range env {
					if strings.HasPrefix(item, "DOCKER_CONTEXT=") {
						t.Fatal("ambient context remained effective")
					}
				}
				if tc.unavailable {
					return nil, errors.New("isolated socket unavailable")
				}
				// The probe answers honestly: the per-user socket is always
				// a Rootless daemon; hostRootful marks a host whose only
				// daemon is System Docker; rootlessAt overrides a daemon's
				// answer at one exact endpoint.
				security := ""
				if args[1] == "unix://"+rootlessSocketPath(1000) && !tc.hostRootful {
					security = `"name=rootless"`
				}
				if tc.rootlessAt != "" && args[1] == tc.rootlessAt {
					security = `"name=rootless"`
				}
				if tc.endpoint != "" && tc.endpoint != args[1] {
					// A second engine is never probed.
					t.Fatalf("probed unselected engine %s", args[1])
				}
				return []byte(`{"ID":"verified-daemon","SecurityOptions":[` + security + `]}`), nil
			}
			got, err := resolveDockerEngine(context.Background(), "docker", tc.selection, run, func(k string) string { return tc.env[k] }, 1000)
			if (err != nil) != tc.invalid {
				t.Fatalf("observation=%+v err=%v", got, err)
			}
			if len(probes) > 1 || (len(probes) == 1 && probes[0] != tc.endpoint) {
				t.Fatalf("engine fallback: %v", probes)
			}
			if !tc.invalid && (!got.Verified || got.Endpoint != tc.endpoint || got.SelectionOrigin != tc.wantOrigin || (tc.wantMode != "" && got.Mode != tc.wantMode)) {
				t.Fatalf("wrong observed engine: %+v", got)
			}
		})
	}
}

func TestHostDefaultEndpointUsesTheActiveEngineOfTheHost(t *testing.T) {
	originalSocketProbe, originalCapability := rootlessSocketExists, rootlessCapable
	t.Cleanup(func() {
		rootlessSocketExists, rootlessCapable = originalSocketProbe, originalCapability
	})
	const rootless, system = "/run/user/1000/docker.sock", "/var/run/docker.sock"
	for _, tc := range []struct {
		name    string
		uid     int
		sockets map[string]bool
		capable bool
		want    string
	}{
		{name: "rootless-socket-present", uid: 1000, sockets: map[string]bool{rootless: true}, capable: true, want: "unix://" + rootless},
		{name: "system-only-host-socket-absent-namespace-denied", uid: 1000, sockets: map[string]bool{rootless: false, system: true}, capable: false, want: "unix://" + system},
		{name: "rootless-capable-host-without-socket-keeps-rootless-default-fail-closed", uid: 1000, sockets: map[string]bool{rootless: false, system: true}, capable: true, want: "unix://" + rootless},
		{name: "root-session-has-no-per-user-socket", uid: 0, sockets: map[string]bool{system: true}, capable: true, want: "unix://" + system},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rootlessSocketExists = func(path string) bool { return tc.sockets[path] }
			rootlessCapable = func() bool { return tc.capable }
			if got := hostDefaultEndpoint(tc.uid); got != tc.want {
				t.Fatalf("hostDefaultEndpoint(%d) = %s, want %s", tc.uid, got, tc.want)
			}
		})
	}
}

func TestBoundDockerAllCommandPathsIgnoreChangedAmbientContext(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "docker")
	log := filepath.Join(dir, "calls")
	script := `#!/bin/sh
set -eu
printf '%s|%s|%s\n' "$*" "$DOCKER_HOST" "${DOCKER_CONTEXT-unset}" >> "$CALL_LOG"
printf 'observed\n'
`
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CALL_LOG", log)
	t.Setenv("DOCKER_CONTEXT", "system")
	t.Setenv("DOCKER_HOST", "unix:///var/run/docker.sock")
	c := NewDockerCLIBackend(bin, DockerEngineObservation{Endpoint: "unix:///run/user/1000/docker.sock", Mode: "rootless", Verified: true, DaemonID: "original"})
	ctx := context.Background()
	if _, err := c.DirectOutput(ctx, "info"); err != nil {
		t.Fatal(err)
	}
	if err := c.UpProject(ctx, "owned", "compose.yaml", "runtime.env"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.outputProjectFilesEnv(ctx, "owned", dir, map[string]string{"WORKLOAD_VALUE": "valid"}, []string{"compose.yaml"}, "config"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.directBinary(ctx, nil, "volume", "ls"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.outputProjectFilesEnv(ctx, "owned", dir, map[string]string{"DOCKER_HOST": "unix:///var/run/docker.sock"}, []string{"compose.yaml"}, "up"); err == nil {
		t.Fatal("application selected another daemon")
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if !strings.HasPrefix(line, "--host unix:///run/user/1000/docker.sock ") || !strings.HasSuffix(line, "|unix:///run/user/1000/docker.sock|unset") {
			t.Fatalf("command escaped selected rootless engine: %s", line)
		}
	}
}
