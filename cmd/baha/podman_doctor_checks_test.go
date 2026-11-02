package main

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// Podman/quadlet readiness checks must exist as two named checks so the native
// Podman acceptance can grep "OK podman" and "OK quadlet". Which provider is
// installed is irrelevant here — the acceptance contract is that the check
// names are emitted for a Podman target.
func TestPodmanRuntimeDoctorChecksEmitsPodmanAndQuadlet(t *testing.T) {
	checks := PodmanRuntimeDoctorChecks(context.Background(), deployment.ResolvedTarget{Name: "podman-ci", RuntimeProvider: "podman"})
	if len(checks) != 2 {
		t.Fatalf("len(checks) = %d, want 2 (%+v)", len(checks), checks)
	}
	names := map[string]bool{}
	for _, c := range checks {
		names[c.Name] = true
	}
	if !names["podman"] || !names["quadlet"] {
		t.Fatalf("missing podman/quadlet checks: %+v", checks)
	}
}

// The control-plane doctor must NOT emit a podman/quadlet check for a Docker
// target: the Docker acceptance asserts every emitted check is ready and never
// expects a podman check, so a stray failing podman check would break it.
func TestCollectControlPlaneDoctorChecksOmitsPodmanForDockerTarget(t *testing.T) {
	target := configureTestTarget(t) // docker default target
	if target.RuntimeProvider != "docker" {
		t.Fatalf("fixture target provider = %q, want docker", target.RuntimeProvider)
	}
	checks := collectControlPlaneDoctorChecks(context.Background())
	for _, c := range checks {
		if c.Name == "podman" || c.Name == "quadlet" {
			t.Fatalf("Docker-target doctor emitted a podman check: %+v", checks)
		}
	}
}

// The control-plane doctor must NOT emit a Docker engine check when the
// target runtime provider is not docker. On a pure Podman host there is no
// Docker socket, so an unconditional Docker-engine probe would fail and
// corrupt the doctor output. The fix gates the Docker-engine check on
// target.RuntimeProvider == "docker".
func TestCollectControlPlaneDoctorChecksOmitsDockerEngineForPodmanTarget(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: "podman-ci",
		Access: map[string]deployment.AccessDefinition{
			"local-podman": {Provider: "local", Reference: "local"},
		},
		Targets: map[string]deployment.TargetDefinition{
			"podman-ci": {
				Runtime:   deployment.RuntimeDefinition{Provider: "podman"},
				Access:    deployment.TargetAccess{Reference: "local-podman"},
				Scope:     "local",
			},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	checks := collectControlPlaneDoctorChecks(context.Background())
	for _, c := range checks {
		if c.Name == "Docker engine" {
			t.Fatalf("podman-target doctor emitted a Docker-engine check: %+v (Docker sockets do not exist on pure Podman hosts)", checks)
		}
	}
}
