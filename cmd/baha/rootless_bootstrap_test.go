package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestRootlessBootstrapEligibilityHonorsExplicitSelection(t *testing.T) {
	base := deployment.ResolvedTarget{Name: "local", RuntimeProvider: "docker", AccessProvider: "local"}
	env := func(string) string { return "" }
	cases := []struct {
		name string
		edit func(*deployment.ResolvedTarget)
		env  func(string) string
		want bool
	}{
		{"unconfigured local docker", func(*deployment.ResolvedTarget) {}, env, true},
		{"rootful mode", func(x *deployment.ResolvedTarget) { x.DockerMode = "rootful" }, env, false},
		{"explicit rootless endpoint", func(x *deployment.ResolvedTarget) { x.DockerEndpoint = "unix:///run/user/1000/docker.sock" }, env, false},
		{"explicit docker context", func(x *deployment.ResolvedTarget) { x.DockerContext = "rootless" }, env, false},
		{"podman", func(x *deployment.ResolvedTarget) { x.RuntimeProvider = "podman" }, env, false},
		{"remote target", func(x *deployment.ResolvedTarget) { x.AccessProvider = "ssh" }, env, false},
		{"docker host env", func(*deployment.ResolvedTarget) {}, func(k string) string {
			if k == "DOCKER_HOST" {
				return "unix:///tmp/docker.sock"
			}
			return ""
		}, false},
		{"docker context env", func(*deployment.ResolvedTarget) {}, func(k string) string {
			if k == "DOCKER_CONTEXT" {
				return "custom"
			}
			return ""
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := base
			tc.edit(&target)
			got := rootlessBootstrapEligible(target, 1000, tc.env)
			if got != tc.want {
				t.Fatalf("eligible=%t, want %t", got, tc.want)
			}
		})
	}
	if rootlessBootstrapEligible(base, 0, env) {
		t.Fatal("root user must never receive automatic rootless repair")
	}
}

func TestRootlessBootstrapExplicitTargetNeverTouchesHost(t *testing.T) {
	target := deployment.ResolvedTarget{Name: "remote", RuntimeProvider: "docker", AccessProvider: "ssh"}
	if err := maybeRepairRootlessDocker(t.Context(), nil, nil, target); err != nil {
		t.Fatalf("remote Target must not trigger local Docker setup: %v", err)
	}
}

func TestRootlessSetupConsentNeverAssumesYes(t *testing.T) {
	for _, tc := range []struct {
		name        string
		input       string
		interactive bool
		wantOK      bool
	}{
		{"accept explicitly", "y\n", true, true},
		{"decline explicitly", "n\n", true, false},
		{"default no", "\n", true, false},
		{"unattended yes is not consent", "y\n", false, false},
		{"end of input", "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			err := requireRootlessSetupConsent(context.Background(), strings.NewReader(tc.input), &output, tc.interactive)
			if (err == nil) != tc.wantOK {
				t.Fatalf("consent err=%v wantOK=%t", err, tc.wantOK)
			}
			if tc.interactive && !strings.Contains(output.String(), "Apply these Rootless Docker setup changes?") {
				t.Fatalf("explicit prompt missing: %q", output.String())
			}
			if !tc.interactive && output.Len() != 0 {
				t.Fatalf("noninteractive prompt leaked: %q", output.String())
			}
		})
	}
}
