package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/onboardingbundle"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	testruntime "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
)

// TestWorkstationOnboardingRemoteNativeInCI is deliberately a mandatory real
// native proof, not an HTTP mock. Its later cells must never be marked PASS by
// skipping a missing integration fixture. The CI proof reader also rejects a
// successful Go package exit when a cell was absent.
func TestWorkstationOnboardingRemoteNativeInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_ONBOARDING_NATIVE_ACCEPTANCE") != "1" {
		t.Skip("native workstation onboarding qualification is not requested")
	}
	kind := strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME"))
	if kind != "docker" && kind != "podman" {
		t.Fatalf("native onboarding requires Docker or Podman, got %q", kind)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	ensureRuntimeIntegrationTrustPlane(t, ctx)
	name := runtimeIntegrationTrustPlaneTargetName
	if name == "" || !strings.HasPrefix(name, "w169-ci-bootstrap-") {
		t.Fatalf("isolated Core fixture unavailable: %q", name)
	}

	t.Run("CoreReady", func(t *testing.T) {
		provider, err := testruntime.Resolve(ctx)
		if err != nil {
			t.Fatal(err)
		}
		containers, err := provider.ListRuntimeContainers(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, container := range containers {
			if container.Project == bhruntime.SharedProjectName(name) && container.Running {
				return
			}
		}
		t.Fatalf("no running owned Core services on native %s", kind)
	})

	var invitation string
	invitationRoot := t.TempDir()
	t.Run("Invitation", func(t *testing.T) {
		previousPasswordReader := readOnboardInitialPasswordForInvite
		readOnboardInitialPasswordForInvite = func(io.Writer, *bufio.Reader) ([]byte, error) {
			return []byte("native-ci-temporary-password"), nil
		}
		t.Cleanup(func() { readOnboardInitialPasswordForInvite = previousPasswordReader })
		owned := filepath.Join(invitationRoot, "private")
		if err := os.Mkdir(owned, 0o700); err != nil {
			t.Fatal(err)
		}
		invitation = filepath.Join(owned, "invite.json")
		var out bytes.Buffer
		err := runOnboardInvite(ctx, []string{
			"--target", name,
			"--name", "CI Workstation User",
			"--login", "ci-workstation-user",
			"--email", "ci-workstation@example.invalid",
			"--station", "ci-workstation-native",
			"--certificate-ttl", "5y",
			"--output", invitation,
		}, &out)
		if err != nil {
			t.Fatalf("Core cannot issue an invitation without hidden manual endpoint setup: %v", err)
		}
		b, err := onboardingbundle.Read(invitation, time.Now())
		if err != nil {
			t.Fatalf("invalid native private invitation: %v", err)
		}
		if b.RuntimeProvider != kind || b.CoreURL == "" || b.CoreInstallationID == "" {
			t.Fatalf("invitation does not identify the native Core and runtime")
		}
	})

	// Switch to a fresh workstation state root; no Docker installation or
	// owner credentials from the Core fixture may be reused by the client.
	workstationRoot := t.TempDir()
	// The invitation importer rejects existing public state roots. Make the
	// fixture's intended private mode explicit instead of relying on runner
	// umask or temporary-directory implementation details.
	if err := os.Chmod(workstationRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	workstationTarget := ""
	workloadDir := t.TempDir()
	applied := false
	t.Run("WorkstationImport", func(t *testing.T) {
		if invitation == "" {
			t.Fatal("Core did not create an invitation")
		}
		t.Setenv("BASEHARBOR_STATE_DIR", workstationRoot)
		t.Setenv("BASEHARBOR_TARGET", "")
		var output bytes.Buffer
		if err := runOnboardImport(ctx, []string{invitation}, &output); err != nil {
			t.Fatalf("fresh workstation cannot enroll against the live Core: %v", err)
		}
		cfg, err := deployment.LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		for _, target := range cfg.TargetNames() {
			resolved, err := cfg.ResolveTarget(target, "")
			if err == nil && resolved.AccessProvider == "baseharbor-core" {
				workstationTarget = target
			}
		}
		if workstationTarget == "" {
			t.Fatal("import did not configure a remotely authenticated Core Target")
		}
	})
	t.Run("RemoteDeployment", func(t *testing.T) {
		if workstationTarget == "" {
			t.Fatal("fresh workstation did not complete remote import")
		}
		t.Setenv("BASEHARBOR_STATE_DIR", workstationRoot)
		t.Setenv("BASEHARBOR_TARGET", workstationTarget)
		t.Chdir(workloadDir)
		if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0o600); err != nil {
			t.Fatal(err)
		}
		manifest := application.WithWorkloadComponents(application.New("onboard-native", "dev", false, false, false), "api")
		// A fresh remote app has no local application identity: the Core assigns
		// it during registration. Do not preseed the local Store from application.New.
		if err := os.WriteFile("baseharbor.yaml", []byte(manifest.AuthoredYAML()), 0o600); err != nil {
			t.Fatal(err)
		}
		// The ordinary repository journey must register through the owning Core,
		// configure the app and infer its name/environment from the manifest.
		// Noninteractive clients still need the exact candidate approval.
		for _, args := range [][]string{
			{"app", "init"},
			{"app", "plan", "onboard-native", "--environment", "dev", "--json"},
			{"app", "list", "--json"},
			{"up"},
			{"app", "status", "onboard-native", "--environment", "dev", "--json"},
			{"app", "doctor", "onboard-native", "--environment", "dev", "--json"},
		} {
			if args[0] == "up" {
				// The first request must fail closed before recording the candidate.
				// The server supplies the hash of its exact verified source, which
				// is then explicitly confirmed by this CI automation.
				var output, errorsOut bytes.Buffer
				attempt := runWithIO(ctx, append([]string{"--no-input"}, args...), &output, &errorsOut)
				var denied *machine.Error
				if !errors.As(attempt, &denied) || denied.CauseCode != "candidate_confirmation_required" {
					t.Fatalf("unconfirmed remote candidate was not refused with a typed consent challenge: %v", attempt)
				}
				match := regexp.MustCompile(` --confirm-candidate ([0-9a-f]{64})$`).FindStringSubmatch(denied.Next)
				if len(match) != 2 {
					t.Fatalf("Core did not supply an exact candidate confirmation hash: %q", denied.Next)
				}
				args = append(args, "--confirm-candidate", match[1])
			}
			var stdout, stderr bytes.Buffer
			if err := runWithIO(ctx, append([]string{"--no-input"}, args...), &stdout, &stderr); err != nil {
				t.Fatalf("remote CLI operation %v failed over the enrolled device: %v", args, err)
			}
			if len(args) > 1 && args[1] == "status" {
				var observed application.StatusResult
				if err := json.Unmarshal(stdout.Bytes(), &observed); err != nil {
					t.Fatalf("remote Core status is not valid JSON: %v\n%s", err, stdout.String())
				}
				if !observed.Ready || observed.State != "running" || observed.Application != "onboard-native" {
					t.Fatalf("remote Core did not observe the deployed workload as ready: state=%q ready=%t application=%q checks=%+v", observed.State, observed.Ready, observed.Application, observed.Checks)
				}
			}
			if len(args) > 1 && args[1] == "plan" {
				var observed applicationLifecyclePlan
				if err := json.Unmarshal(stdout.Bytes(), &observed); err != nil || observed.Application != "onboard-native" || observed.Environment != "dev" {
					t.Fatalf("remote Core plan does not describe the selected application: %v\n%s", err, stdout.String())
				}
			}
			if len(args) > 1 && args[1] == "list" {
				var observed machineApplicationListResult
				if err := json.Unmarshal(stdout.Bytes(), &observed); err != nil || observed.ContractVersion != machine.ContractVersion {
					t.Fatalf("remote Core application list has no valid contract: %v\n%s", err, stdout.String())
				}
				found := false
				for _, item := range observed.Deployments {
					if item.Application == "onboard-native" && item.Environment == "dev" && item.Target == workstationTarget {
						found = true
					}
				}
				if !found {
					t.Fatalf("remote Core application list omitted the selected Target deployment: %s", stdout.String())
				}
			}
			if len(args) > 1 && args[1] == "doctor" {
				var observed applicationDoctorResult
				if err := json.Unmarshal(stdout.Bytes(), &observed); err != nil || !observed.Healthy || observed.Target != workstationTarget || observed.Application != "onboard-native" {
					t.Fatalf("remote Core doctor did not report a healthy selected Target application: %v\n%s", err, stdout.String())
				}
			}
		}
		applied = true
	})
	t.Run("RemoteDestroy", func(t *testing.T) {
		if !applied || workstationTarget == "" {
			t.Fatal("remote application was not deployed by this test")
		}
		t.Setenv("BASEHARBOR_STATE_DIR", workstationRoot)
		t.Setenv("BASEHARBOR_TARGET", workstationTarget)
		t.Chdir(workloadDir)
		var stdout, stderr bytes.Buffer
		destroyCtx := withApplicationInput(ctx, strings.NewReader("y\ny\n"))
		if err := runWithIO(destroyCtx, []string{"app", "destroy", "onboard-native", "--yes"}, &stdout, &stderr); err != nil {
			t.Fatalf("remote application destroy failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
		}
		if _, err := deployment.LoadConfig(); err != nil {
			t.Fatalf("remote destroy damaged the workstation's target configuration: %v", err)
		}
	})
}
