package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestRemoteMutationArgumentsFailClosed(t *testing.T) {
	tests := []struct {
		operation string
		args      []string
		accepted  bool
	}{
		{"apply", []string{"demo"}, true},
		{"apply", []string{"demo", "--environment", "dev", "--skip-memory-preflight"}, true},
		{"apply", []string{"demo", "--confirm-candidate", "bogus"}, false},
		{"apply", []string{}, false},
		{"apply", []string{"demo", "--environment", "prod"}, false},
		{"update", []string{"demo"}, false},
		{"update", []string{"demo", "--no-backup"}, true},
		{"update", []string{"demo", "--backup-password-file", "local.txt"}, false},
		{"destroy", []string{"demo"}, false},
		{"destroy", []string{"demo", "--yes"}, true},
		{"destroy", []string{"demo", "--yes", "--full-reset"}, true},
		{"destroy", []string{"demo", "--full-reset"}, false},
		{"destroy", []string{"demo", "--yes", "--delete-data"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.operation+"/"+strings.Join(tc.args, "-"), func(t *testing.T) {
			_, _, payload, err := remoteMutationArgs(tc.operation, tc.args)
			if tc.accepted && (err != nil || len(payload) == 0) {
				t.Fatalf("expected safe remote mutation args: %v", err)
			}
			if !tc.accepted && err == nil {
				t.Fatal("unsupported or unapproved remote mutation was accepted")
			}
		})
	}
}

func TestRemoteMutationRequiresVerifiedCoreWithoutLocalFallback(t *testing.T) {
	_, _, node := remoteApplicationCoreFixtureEmpty(t)
	t.Setenv("BASEHARBOR_CONNECTION_PROFILE", "")
	for _, tc := range []struct {
		operation string
		args      []string
	}{
		{"apply", []string{"demo"}},
		{"update", []string{"demo", "--no-backup"}},
		{"destroy", []string{"demo", "--yes"}},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			var output bytes.Buffer
			err := remoteApplicationMutation(context.Background(), node, tc.operation, tc.args, &output)
			if err == nil || !strings.Contains(err.Error(), "BASEHARBOR_CONNECTION_PROFILE") {
				t.Fatalf("remote operation did not fail closed without verified Core: %v", err)
			}
			if output.Len() > 0 {
				t.Fatalf("unverified remote mutation emitted result: %q", output.String())
			}
		})
	}
}

func TestRemoteApplicationCLIEntrypointsNeverRunLocalRuntime(t *testing.T) {
	_, _, node := remoteApplicationCoreFixtureEmpty(t)
	t.Setenv("BASEHARBOR_TARGET", node.Name)
	t.Setenv("BASEHARBOR_CONNECTION_PROFILE", "")
	store := application.DefaultStore()
	for _, tc := range []struct {
		name string
		run  func(context.Context, []string, *bytes.Buffer, *bytes.Buffer) error
		args []string
	}{
		{"app-apply", appApplyCommand(store).Run, []string{"demo", "--environment", "dev"}},
		{"app-up", appUpCommand(store).Run, []string{"demo"}},
		{"app-update", appUpdateCommand(store).Run, []string{"demo", "--no-backup"}},
		{"app-destroy", appDestroyCommand(store).Run, []string{"demo", "--yes"}},
		{"root-up", runtimeUpCommandWithInputResolver, []string{"demo", "--environment", "dev"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if err := tc.run(context.Background(), tc.args, &out, &errOut); err == nil ||
				!strings.Contains(err.Error(), "BASEHARBOR_CONNECTION_PROFILE") {
				t.Fatalf("CLI bypassed remote Core trust boundary: %v", err)
			}
			if out.Len() != 0 {
				t.Fatalf("unverified remote operation produced output: %q", out.String())
			}
		})
	}
}
