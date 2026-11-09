package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
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
		{"apply", []string{"demo", "--confirm-candidate="}, false},
		{"apply", []string{"demo", "--confirm-candidate", ""}, false},
		{"apply", []string{"demo", "--provider-consent-file="}, false},
		{"destroy", []string{"demo", "--yes", "--provider-consent-file="}, false},
		{"apply", []string{"demo", "--provider-consent-file=", "--provider-consent-file", "consent.json"}, false},
		{"apply", []string{"demo", "--confirm-candidate", strings.Repeat("a", 64), "--confirm-candidate", strings.Repeat("b", 64)}, false},
		{"apply", []string{"demo", "--confirm-candidate=" + strings.Repeat("a", 64), "--confirm-candidate=" + strings.Repeat("b", 64)}, false},
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
		{"app.stop", []string{"demo"}, true},
		{"app.stop", []string{"demo", "--yes"}, false},
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
		run  func(context.Context, []string, io.Writer, io.Writer) error
		args []string
	}{
		{"app-apply", appApplyCommand(store).Run, []string{"demo", "--environment", "dev"}},
		{"app-up", appUpCommand(store).Run, []string{"demo"}},
		{"app-update", appUpdateCommand(store).Run, []string{"demo", "--no-backup"}},
		{"app-destroy", appDestroyCommand(store).Run, []string{"demo", "--yes"}},
		{"app-down", appDownCommand(store).Run, []string{"demo"}},
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

func TestRemoteProviderConsentIsExplicitAndPlanBound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "consent.json")
	if err := os.WriteFile(path, []byte(`{"plan_id":"plan-current","first":true,"second":true,"delete_data":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, payload, err := remoteMutationArgs("destroy",
		[]string{"demo", "--yes", "--provider-consent-file", path})
	if err != nil {
		t.Fatalf("approved remote plan rejected: %v", err)
	}
	var result struct {
		ProviderChangeConsent struct {
			PlanID string `json:"plan_id"`
			First  bool   `json:"first"`
			Second bool   `json:"second"`
		} `json:"provider_change_consent"`
	}
	if err := json.Unmarshal(payload, &result); err != nil || result.ProviderChangeConsent.PlanID != "plan-current" ||
		!result.ProviderChangeConsent.First || !result.ProviderChangeConsent.Second {
		t.Fatalf("approved current plan was not delivered to Core: %s (%v)", payload, err)
	}
	if _, _, _, err := remoteMutationArgs("update",
		[]string{"demo", "--no-backup", "--provider-consent-file", path}); err == nil {
		t.Fatal("update accepted unsupported provider approval")
	}
	if err := os.WriteFile(path, []byte(`{"plan_id":"plan-current","first":true,"second":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := remoteMutationArgs("destroy",
		[]string{"demo", "--yes", "--provider-consent-file", path}); err == nil {
		t.Fatal("unconfirmed second approval was accepted")
	}
}

func TestRemoteApplyCarriesExactCandidateDigest(t *testing.T) {
	_, _, payload, err := remoteMutationArgs("apply", []string{"demo", "--confirm-candidate", strings.Repeat("a", 64)})
	if err != nil {
		t.Fatalf("explicit remote candidate rejected: %v", err)
	}
	var input struct {
		ConfirmCandidate string `json:"confirm_candidate"`
	}
	if err := json.Unmarshal(payload, &input); err != nil || input.ConfirmCandidate != strings.Repeat("a", 64) {
		t.Fatalf("remote Core lost candidate confirmation: %s (%v)", payload, err)
	}
	if _, _, _, err := remoteMutationArgs("destroy",
		[]string{"demo", "--yes", "--confirm-candidate", strings.Repeat("a", 64)}); err == nil {
		t.Fatal("destroy accepted apply-only candidate confirmation")
	}
}

func TestRemoteExplicitIdempotencyRecovery(t *testing.T) {
	args, key, err := extractRemoteIdempotencyKey([]string{"demo", "--yes", "--idempotency-key", "recovery-123"})
	if err != nil || key != "recovery-123" || len(args) != 2 {
		t.Fatalf("explicit stable recovery key was not preserved: %q %v %v", key, args, err)
	}
	for _, invalid := range [][]string{
		{"demo", "--idempotency-key="},
		{"demo", "--idempotency-key", "../foreign"},
		{"demo", "--idempotency-key", "first", "--idempotency-key", "second"},
		{"demo", "--idempotency-key"},
	} {
		if _, _, err := extractRemoteIdempotencyKey(invalid); err == nil {
			t.Fatalf("unsafe remote recovery key accepted: %v", invalid)
		}
	}
}
