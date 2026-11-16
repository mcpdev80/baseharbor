package main

import (
	"bytes"
	"context"
	"errors"
	"io"
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

func TestRootlessSubIDRangeValidation(t *testing.T) {
	cases := []struct {
		name    string
		content string
		user    string
		want    bool
	}{
		{"valid", "other:100000:65536\nmarcel:231072:65536\n", "marcel", true},
		{"short allocation", "marcel:100000:65535", "marcel", false},
		{"different user", "other:100000:65536", "marcel", false},
		{"invalid start", "marcel:abc:65536", "marcel", false},
		{"overflow", "marcel:4294967290:65536", "marcel", false},
		{"no root mapping", "marcel:0:65536", "marcel", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasRootlessSubIDRange(tc.content, tc.user); got != tc.want {
				t.Fatalf("range accepted=%t want=%t", got, tc.want)
			}
		})
	}
}

func TestApprovedRootlessSetupCommandSequence(t *testing.T) {
	for _, tc := range []struct {
		name    string
		install bool
		uidmap  bool
		want    []string
	}{
		{"fresh host missing uidmap", true, true, []string{
			"sudo apt-get install -y uidmap",
			"dockerd-rootless-setuptool.sh install --force",
			"systemctl --user enable --now docker",
			"docker context use default",
			"docker context use default",
		}},
		{"fresh host with uidmap", true, false, []string{
			"dockerd-rootless-setuptool.sh install --force",
			"systemctl --user enable --now docker",
			"docker context use default",
			"docker context use default",
		}},
		{"existing service", false, false, []string{
			"systemctl --user enable docker",
			"systemctl --user restart docker",
			"docker context use default",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var commands []string
			run := func(_ context.Context, _ io.Reader, _ io.Writer, name string, args ...string) error {
				commands = append(commands, name + " " + strings.Join(args, " "))
				return nil
			}
			lookup := func(name string) (string, error) { return "/usr/bin/" + name, nil }
			if err := runApprovedRootlessSetup(context.Background(), strings.NewReader(""), io.Discard, tc.install, tc.uidmap, run, lookup); err != nil {
				t.Fatal(err)
			}
			if strings.Join(commands, "\n") != strings.Join(tc.want, "\n") {
				t.Fatalf("commands=%v want=%v", commands, tc.want)
			}
		})
	}
}

func TestRootlessSetupPartialFailuresNeverRunUnapprovedFallback(t *testing.T) {
	for _, tc := range []struct {
		name string
		fail string
		want []string
	}{
		{"uidmap install fails", "sudo", []string{"sudo"}},
		{"setuptool fails", "dockerd-rootless-setuptool.sh", []string{"sudo", "dockerd-rootless-setuptool.sh", "docker"}},
		{"service start fails", "systemctl", []string{"sudo", "dockerd-rootless-setuptool.sh", "systemctl", "docker"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var called []string
			run := func(_ context.Context, _ io.Reader, _ io.Writer, name string, _ ...string) error {
				called = append(called, name)
				if name == tc.fail {
					return errors.New("injected failure")
				}
				return nil
			}
			lookup := func(name string) (string, error) { return "/usr/bin/" + name, nil }
			if err := runApprovedRootlessSetup(context.Background(), strings.NewReader(""), io.Discard, true, true, run, lookup); err == nil {
				t.Fatal("failure must abort setup")
			}
			if strings.Join(called, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("called=%v want=%v", called, tc.want)
			}
		})
	}
}

func TestRootlessSetupMissingUIDMapAfterInstallFailsClosed(t *testing.T) {
	var called []string
	run := func(_ context.Context, _ io.Reader, _ io.Writer, name string, _ ...string) error {
		called = append(called, name)
		return nil
	}
	lookup := func(name string) (string, error) { return "", errors.New("missing " + name) }
	if err := runApprovedRootlessSetup(context.Background(), strings.NewReader(""), io.Discard, true, true, run, lookup); err == nil || !strings.Contains(err.Error(), "newuidmap") {
		t.Fatalf("expected missing uidmap failure, got %v", err)
	}
	if strings.Join(called, ",") != "sudo" {
		t.Fatalf("unexpected changes after invalid installation: %v", called)
	}
}

func TestRootlessSetupContextRestoreFailureIsReported(t *testing.T) {
	var called []string
	run := func(_ context.Context, _ io.Reader, _ io.Writer, name string, args ...string) error {
		key := name + " " + strings.Join(args, " ")
		called = append(called, key)
		if key == "docker context use default" {
			return errors.New("cannot restore context")
		}
		return nil
	}
	lookup := func(name string) (string, error) { return "/usr/bin/" + name, nil }
	err := runApprovedRootlessSetup(context.Background(), strings.NewReader(""), io.Discard, true, false, run, lookup)
	if err == nil || !strings.Contains(err.Error(), "restore Docker context") {
		t.Fatalf("lost context restore error: %v", err)
	}
	if len(called) < 2 || called[len(called)-1] != "docker context use default" {
		t.Fatalf("context was not restored on error: %v", called)
	}
}
