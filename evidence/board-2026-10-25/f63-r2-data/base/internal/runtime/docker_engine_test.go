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
	for _, tc := range []struct {
		name                   string
		selection              DockerEngineSelection
		env                    map[string]string
		active, endpoint, mode string
		unavailable, invalid   bool
		wantOrigin             string
	}{
		{name: "active-rootless", active: "rootless", endpoint: "unix:///run/user/1000/docker.sock", mode: "rootless", wantOrigin: "active-context"},
		{name: "default-is-rootless", active: "default", endpoint: "unix:///run/user/1000/docker.sock", mode: "rootless", wantOrigin: "rootless-default"},
		{name: "target-endpoint-wins", selection: DockerEngineSelection{Endpoint: "unix:///selected.sock"}, env: map[string]string{"DOCKER_CONTEXT": "other", "DOCKER_HOST": "unix:///wrong.sock"}, endpoint: "unix:///selected.sock", mode: "rootless", wantOrigin: "target-endpoint"},
		{name: "target-context-wins", selection: DockerEngineSelection{Context: "chosen"}, env: map[string]string{"DOCKER_CONTEXT": "other"}, endpoint: "unix:///selected.sock", mode: "rootless", wantOrigin: "target-context"},
		{name: "environment-context-wins", env: map[string]string{"DOCKER_CONTEXT": "chosen", "DOCKER_HOST": "unix:///wrong.sock"}, endpoint: "unix:///selected.sock", mode: "rootless", wantOrigin: "environment-context"},
		{name: "environment-endpoint", env: map[string]string{"DOCKER_HOST": "unix:///selected.sock"}, endpoint: "unix:///selected.sock", mode: "rootless", wantOrigin: "environment-endpoint"},
		{name: "unreachable-never-falls-back", active: "rootless", endpoint: "unix:///run/user/1000/docker.sock", unavailable: true, invalid: true},
		{name: "rootful-rejected", active: "rootless", endpoint: "unix:///run/user/1000/docker.sock", mode: "rootful", invalid: true},
		{name: "explicit-rootful-maintenance", selection: DockerEngineSelection{Endpoint: "unix:///var/run/docker.sock", Mode: "rootful"}, endpoint: "unix:///var/run/docker.sock", mode: "rootful", wantOrigin: "target-endpoint"},
		{name: "conflicting-target-selection", selection: DockerEngineSelection{Endpoint: "unix:///a.sock", Context: "chosen"}, invalid: true},
		{name: "plaintext-remote-denied", selection: DockerEngineSelection{Endpoint: "tcp://remote:2375"}, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
				security := ""
				if tc.mode == "rootless" {
					security = `"name=rootless"`
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
			if !tc.invalid && (!got.Verified || got.Endpoint != tc.endpoint || got.Mode != tc.mode || got.SelectionOrigin != tc.wantOrigin) {
				t.Fatalf("wrong observed engine: %+v", got)
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
