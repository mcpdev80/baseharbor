package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// This gate deliberately never calls ensureRuntimeIntegrationTrustPlane:
// bootstrap would invalidate the fresh-host, zero-Core acceptance requirement.
func TestCorelessFreshWorkloadRuntimeLifecycle(t *testing.T) {
	if os.Getenv("BASEHARBOR_CORELESS_ACCEPTANCE") != "1" {
		t.Skip("requires BASEHARBOR_CORELESS_ACCEPTANCE=1 on an isolated rootless Docker/Podman host")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	ctx = machineNoninteractiveContext(ctx)
	// This direct helper call stands in for the local CLI boundary. In
	// production the CLI adds this marker before authorizing local prod work.
	ctx = withLocalInvocation(ctx, "cli")
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTargetEmpty(t)
	engine := "docker"
	format := "{{json .SecurityOptions}}"
	if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
		engine, format = "podman", "{{.Host.Security.Rootless}}"
		cfg, err := deployment.LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		definition := cfg.Targets[target.Name]
		definition.Runtime.Provider = engine
		cfg.Targets[target.Name] = definition
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		target, err = cfg.ResolveTarget("", "")
		if err != nil {
			t.Fatal(err)
		}
	}
	security, err := exec.CommandContext(ctx, engine, "info", "--format", format).Output()
	if err != nil || (engine == "docker" && !strings.Contains(string(security), "rootless")) || (engine == "podman" && strings.TrimSpace(string(security)) != "true") {
		t.Fatal("acceptance requires a verified rootless runtime")
	}
	runtime, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	assertCorelessHost(t, ctx, runtime, target)
	root := t.TempDir()
	t.Chdir(root)
	environment := strings.TrimSpace(os.Getenv("BASEHARBOR_CORELESS_ENVIRONMENT"))
	if environment == "" {
		environment = "dev"
	}
	if environment != "dev" && environment != "prod" {
		t.Fatal("coreless acceptance environment must be dev or prod")
	}
	authored := fmt.Sprintf("version: 1\napp:\n  name: coreless-http\n  environment: %s\n", environment)
	if err := os.WriteFile("baseharbor.yaml", []byte(authored), 0o600); err != nil {
		t.Fatal(err)
	}
	var missingOut bytes.Buffer
	if err := runWithIO(ctx, []string{"up", "--yes", "--skip-memory-preflight"}, &missingOut, &missingOut); err == nil {
		t.Fatal("empty repository reported successful deployment")
	}
	if _, err := os.Stat(filepath.Join(root, ".baseharbor", "apps")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing workload initialized identity: %v", err)
	}
	t.Logf("missing workload refused before initialization: %s", strings.TrimSpace(missingOut.String()))
	assertCorelessHost(t, ctx, runtime, target)
	if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "init", "--quick", "--json"}, &out, &out); err != nil {
		t.Fatalf("minimal initialization: %v\n%s", err, out.String())
	}
	initialized, err := resolveApplication(ctx, application.Store{}, nil, "plan")
	if err != nil {
		t.Fatal(err)
	}
	m := initialized.Manifest
	if m.ApplicationID == "" || m.Services.SQL {
		t.Fatal("minimal initialization allocated implicit SQL or no stable identity")
	}
	submission, _ := candidateSubmissionForResolvedApp(ctx, initialized)
	confirmDigest := delivery.CandidateConsentDigest(submission)
	if confirmDigest == "" {
		t.Fatal("minimal repository candidate did not produce a confirmation digest")
	}
	after, err := os.ReadFile("baseharbor.yaml")
	if err != nil || string(after) != authored {
		t.Fatal("initialization rewrote authored minimal intent")
	}
	t.Logf("runtime=%s application_id=%s target=%s fresh Core absent", engine, m.ApplicationID, target.Name)
	assertCorelessHost(t, ctx, runtime, target)
	destroyed := false
	cleanupRequired := false
	t.Cleanup(func() {
		if destroyed || !cleanupRequired {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cleanupCancel()
		if err := destroySelectiveApplicationWithConsent(machineNoninteractiveContext(cleanupCtx)); err != nil {
			t.Errorf("isolated app cleanup: %v", err)
		}
	})
	// `up` may persist a configured deployment record while initializing
	// runtime intent before it reaches the deployment-consent boundary. That
	// changes the candidate, so the digest computed immediately after `app init`
	// is deliberately stale for this first attempt. Verify the stale digest is
	// refused without advancing that configuration record to an applying or
	// consented state, then bind only the exact digest printed by the refusal.
	if _, found, err := deployment.FindDeployment(target.Name, m.ApplicationID, m.Environment); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatal("fresh coreless app unexpectedly has deployment state before up")
	}
	if err := ensureRepositoryDeploymentInputsForUp(ctx, strings.NewReader(""), io.Discard, runtimeUpOptions{Yes: true}); err != nil {
		t.Fatalf("prepare deployment inputs before candidate refusal: %v", err)
	}
	beforeRefusal, found, err := deployment.FindDeployment(target.Name, m.ApplicationID, m.Environment)
	if err != nil || !found || beforeRefusal.Observed.State != "configured" || beforeRefusal.Consent != nil {
		t.Fatalf("runtime input preparation did not leave an unconsented configured record: found=%t state=%q consent=%+v err=%v", found, beforeRefusal.Observed.State, beforeRefusal.Consent, err)
	}
	cleanupRequired = true
	staleUp := []string{"up", "--yes", "--skip-memory-preflight", "--confirm-candidate", confirmDigest}
	var staleOut bytes.Buffer
	staleErr := runWithIO(ctx, staleUp, &staleOut, &staleOut)
	if staleErr == nil || !strings.Contains(staleErr.Error(), "candidate confirm digest does not match") {
		t.Fatalf("stale pre-up digest was not refused at candidate consent: %v\n%s", staleErr, staleOut.String())
	}
	currentDigest := extractCandidateConfirmDigest(staleOut.String())
	if currentDigest == "" || currentDigest == confirmDigest || !strings.Contains(staleOut.String(), "Re-run with --confirm-candidate "+currentDigest) {
		t.Fatalf("up did not publish a new exact candidate confirmation digest: %s", staleOut.String())
	}
	configured, found, err := deployment.FindDeployment(target.Name, m.ApplicationID, m.Environment)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !reflect.DeepEqual(configured, beforeRefusal) {
		t.Fatalf("refused stale candidate consent advanced deployment state: found=%t state=%q consent=%+v", found, configured.Observed.State, configured.Consent)
	}
	t.Log("stale pre-up candidate digest refused; configured runtime intent remains unconsented")

	confirmedUp := []string{"up", "--yes", "--skip-memory-preflight", "--confirm-candidate", currentDigest}
	// --yes authorizes attempting the stop; the provider consequence plan still
	// requires its own current acknowledgement, which this first stop omits.
	for _, command := range [][]string{{"app", "plan", "--json"}, confirmedUp, {"app", "status", "--json"}, {"app", "doctor", "--json"}, {"app", "down", "--yes"}, confirmedUp} {
		out.Reset()
		var operationErr error
		if command[1] == "down" {
			denied := runWithIO(ctx, command, &out, &out)
			var failure *machine.Error
			if !errors.As(denied, &failure) || failure.Code != machine.ErrorApprovalRequired {
				t.Fatalf("native application stop lacks truthful missing-acknowledgement semantics: %v", denied)
			}
			current, err := resolveApplication(ctx, application.Store{}, nil, "status")
			if err != nil {
				t.Fatal(err)
			}
			actual, _, err := collectResolvedApplicationStatus(ctx, current)
			if err != nil || !actual.Ready {
				t.Fatal("denied application stop changed actual workload readiness")
			}
			operationErr = stopSelectiveApplicationWithConsent(ctx, &out)
		} else {
			operationErr = runWithIO(ctx, command, &out, &out)
		}
		if err := operationErr; err != nil {
			for cause := err; cause != nil; cause = errors.Unwrap(cause) {
				t.Logf("secret-free workload failure: %v", cause)
			}
			if inventory, inspectErr := runtime.ListRuntimeContainers(ctx); inspectErr == nil {
				t.Logf("workload inventory: %+v", inventory)
			}
			t.Fatalf("%v: %v\n%s", command, err, out.String())
		}
		assertCorelessHost(t, ctx, runtime, target)
		if len(command) > 1 && command[1] == "down" {
			stopped, err := resolveApplication(ctx, application.Store{}, nil, "status")
			if err != nil {
				t.Fatal(err)
			}
			status, _, err := collectResolvedApplicationStatus(ctx, stopped)
			if err != nil || status.State != "stopped" || status.Ready {
				t.Fatalf("approved application down did not report the stopped workload: %+v %v", status, err)
			}
			record, found, recordErr := deployment.FindDeployment(target.Name, m.ApplicationID, m.Environment)
			if recordErr != nil || !found || record.Observed.State != "runtime-retired" || record.Observed.Ready {
				t.Fatalf("application down did not preserve its stopped deployment owner: found=%t state=%q ready=%t err=%v", found, record.Observed.State, record.Observed.Ready, recordErr)
			}
			files, err := application.ExistingRuntimeFiles(stopped.Store, stopped.Manifest)
			if err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(files.Compose)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(files.Compose, []byte("services:\n  foreign:\n    image: redis:latest\n"), 0600); err != nil {
				t.Fatal(err)
			}
			out.Reset()
			rejected := runWithIO(ctx, []string{"app", "up", "--skip-memory-preflight"}, &out, &out)
			if err := os.WriteFile(files.Compose, original, 0600); err != nil {
				t.Fatal(err)
			}
			if rejected == nil {
				t.Fatal("damaged/foreign runtime accepted for mutation")
			}
			assertCorelessHost(t, ctx, runtime, target)
			t.Logf("stopped preserved; changed managed Compose refused: %v", rejected)
		}
	}
	t.Log("Actual top-level baha up and stopped-workload reapply preserve a fresh zero-Core host and the canonical application identity.")
	resolved, err := resolveApplication(ctx, application.Store{}, nil, "status")
	if err != nil || resolved.Manifest.ApplicationID != m.ApplicationID {
		t.Fatalf("application identity drifted: %v", err)
	}
	status, _, err := collectResolvedApplicationStatus(ctx, resolved)
	if err != nil || !status.Ready {
		t.Fatalf("real workload not verified: %+v %v", status, err)
	}
	files, err := application.ExistingRuntimeFiles(resolved.Store, m)
	if err != nil {
		t.Fatal(err)
	}
	workload, found, err := application.MaterializeWorkload(root, m, files)
	if err != nil || !found {
		t.Fatalf("workload missing: %v", err)
	}
	body, err := runtime.ExecProjectFiles(ctx, workload.Project, root, "api", []string{workload.Compose, workload.Override}, "python", "-c", "import urllib.request; print(urllib.request.urlopen('http://127.0.0.1:8080/').read().decode())")
	if err != nil || strings.TrimSpace(body) != "ready" {
		t.Fatalf("real HTTP probe: %q %v", body, err)
	}
	qualifyActualRuntimeExplorerConsent(t, ctx, resolved)
	if environment == "prod" {
		qualifyActualLocalStdioWorkloadAuthority(t, ctx, target, m)
	}
	resolved, err = resolveApplication(ctx, application.Store{}, nil, "destroy")
	if err != nil {
		t.Fatalf("resolve application for destroy: %v", err)
	}
	plan, err := buildApplicationDestructionPlan(ctx, resolved, true)
	if err != nil {
		t.Fatalf("build destruction plan: %v", err)
	}
	ack := corelessAcceptanceDestroyConsent(plan)
	if err := runWithIO(withProviderChangeConsent(ctx, ack), []string{"app", "destroy", "--yes"}, &out, &out); err != nil {
		t.Fatalf("destroy: %v\n%s", err, out.String())
	}
	destroyed = true
	assertCorelessHost(t, ctx, runtime, target)
	if _, found, err := deployment.FindDeployment(target.Name, m.ApplicationID, m.Environment); err != nil || found {
		t.Fatalf("full destroy retained deployment owner: found=%t err=%v", found, err)
	}
	out.Reset()
	if err := runWithIO(ctx, []string{"app", "destroy", "--yes"}, &out, &out); err != nil {
		t.Fatalf("repeated destroy of already absent application should be idempotent: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Application already absent") || !strings.Contains(out.String(), "source files were kept") {
		t.Fatalf("repeated destroy did not report an idempotent no-op: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "baseharbor.yaml")); err != nil {
		t.Fatalf("destroy removed authored intent: %v", err)
	}
}

func corelessAcceptanceDestroyConsent(plan applicationDestructionPlan) *application.ProviderChangeConsent {
	return &application.ProviderChangeConsent{
		PlanID:     plan.Scope.PlanID,
		First:      true,
		Second:     true,
		DeleteData: true,
	}
}

func TestCorelessAcceptanceDestroyConsentBindsCurrentDestructivePlan(t *testing.T) {
	resolved := retainedNativeBackendFixture(t)
	plan, err := buildApplicationDestructionPlan(t.Context(), resolved, true)
	if err != nil {
		t.Fatal(err)
	}
	ack := corelessAcceptanceDestroyConsent(plan)
	if !plan.Scope.Destructive || !ack.DeleteData || ack.PlanID != plan.Scope.PlanID {
		t.Fatalf("acceptance destroy did not bind the destructive plan and data consent: plan=%+v ack=%+v", plan.Scope, ack)
	}
	if err := application.ValidateProviderImpactScopeConsent(plan.Scope, *ack); err != nil {
		t.Fatalf("current acceptance destroy consent rejected: %v", err)
	}
	stale := *ack
	stale.PlanID = "stale-plan"
	if err := application.ValidateProviderImpactScopeConsent(plan.Scope, stale); err == nil {
		t.Fatal("stale acceptance destroy consent was accepted")
	}
}

func assertCorelessHost(t *testing.T, ctx context.Context, runtime bhruntime.RuntimeProvider, target deployment.ResolvedTarget) {
	t.Helper()
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coreinstallation.Load(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected Core installation state: %v", err)
	}
	containers, err := runtime.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	coreCount := 0
	for _, container := range containers {
		if container.Project == targetRuntimeProjectName(target) || container.Project == bhruntime.SharedProjectName(target.Name+"-core") {
			coreCount++
			t.Fatalf("unexpected Core container: %s (%s)", container.ID, container.Service)
		}
	}
	t.Logf("live runtime inventory: Core containers=%d total containers=%d", coreCount, len(containers))
}

const corelessHTTPWorkload = `services:
  api:
    image: docker.io/library/python:3.13-alpine
    user: "1000:1000"
    read_only: true
    cap_drop: [ALL]
    security_opt: [no-new-privileges:true]
    tmpfs: [/tmp]
    command: [sh, -ec, "mkdir -p /tmp/www; printf ready >/tmp/www/index.html; exec python -m http.server 8080 --directory /tmp/www"]
    healthcheck:
      test: [CMD, python, -c, "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8080/')"]
      interval: 1s
      timeout: 2s
      retries: 30
`
