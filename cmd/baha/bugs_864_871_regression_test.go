package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestBug864PromptEOFNeverWrites(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := deployment.Config{Version: deployment.ConfigVersion, Prompt: deployment.PromptConfig{Preset: "compact", Position: "before-path", Environment: "critical-only"}}
	for _, existing := range []bool{false, true} {
		if existing {
			if err := cfg.Save(); err != nil {
				t.Fatal(err)
			}
		}
		path, _ := deployment.ConfigPath()
		before, _ := os.ReadFile(path)
		for stage := 0; stage < 9; stage++ {
			err := runPromptWizardInput(cfg, cfg.Prompt, strings.NewReader(strings.Repeat("\n", stage)), io.Discard)
			after, _ := os.ReadFile(path)
			if err == nil || !bytes.Equal(before, after) {
				t.Fatalf("EOF stage %d changed config or succeeded: %v", stage, err)
			}
		}
		// A blank save answer must leave config unchanged; only explicit yes saves.
		for _, save := range []string{"\n", "n\n", "y\n"} {
			var output bytes.Buffer
			err := runPromptWizardInput(cfg, cfg.Prompt, strings.NewReader(strings.Repeat("\n", 8)+save), &output)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output.String(), "configuration saved.") != (save == "y\n") {
				t.Fatalf("wrong confirmation semantics: %s", output.String())
			}
		}
	}
}

func TestBug864PromptRejectsNonTTY(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	old := os.Stdin
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = old; f.Close() })
	if err := runConfigPrompt(context.Background(), nil, io.Discard, io.Discard); err == nil {
		t.Fatal("non-TTY accepted")
	}
	path, _ := deployment.ConfigPath()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("non-TTY wrote config: %v", err)
	}
	if err := runConfigPrompt(context.Background(), []string{"--enable", "--preset", "compact"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestBug868PersistedRecoveryRequiresAbsentCore(t *testing.T) {
	configureTestTarget(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "operator.json")
	original := []byte("operator recovery material")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := persistTargetRecoveryFileReference(ctx, path); err != nil {
		t.Fatal(err)
	}
	a, _, err := preflightNewTargetRecoveryFile(ctx, "")
	if err != nil || a == path {
		t.Fatalf("absent Core: %s %v", a, err)
	}
	b, _, err := preflightNewTargetRecoveryFile(ctx, "")
	if err != nil || a == b {
		t.Fatal("output names reused", err)
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bhruntime.EnsureFilesForProject(root, targetRuntimeProjectName(target), bhruntime.Ports{Postgres: 5432, OpenBao: 8200}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := preflightNewTargetRecoveryFile(ctx, ""); err == nil {
		t.Fatal("active Core recovery reference rotated")
	}
	if _, _, err := preflightNewTargetRecoveryFile(ctx, path); err == nil {
		t.Fatal("explicit existing file accepted")
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "symlink.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if err := persistTargetRecoveryFileReference(ctx, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := preflightNewTargetRecoveryFile(ctx, ""); err == nil {
		t.Fatal("symlink accepted")
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, original) {
		t.Fatal("recovery modified")
	}
}

func TestBug869PortDecisionAndVisibleActivity(t *testing.T) {
	for _, fixed := range []bool{false, true} {
		accept := acceptWorkloadPortFallback
		if fixed {
			accept = acceptFixedWorkloadPortFallback
		}
		ok, err := accept(machineNoninteractiveContext(context.Background()), strings.NewReader(""), io.Discard, "app", 8080, 8082)
		var typed *machine.Error
		if ok || !errors.As(err, &typed) || typed.Code != machine.ErrorPortConflict || !strings.Contains(typed.Next, "--yes") {
			t.Fatalf("unconfirmed remapping: %v %v", ok, err)
		}
		for _, quiet := range []bool{false, true} {
			ctx := cli.WithOutputOptions(withAssumeYes(context.Background(), true), cli.OutputOptions{Quiet: quiet, Plain: true})
			var out bytes.Buffer
			term := cli.NewTerminal(ctx, &out, &out)
			err := term.Activity(ctx, "Starting workload", func(w io.Writer) error {
				ok, err := accept(ctx, strings.NewReader(""), w, "app", 8080, 8082)
				if err != nil || !ok {
					return errors.New("authorized remap rejected")
				}
				cli.ReportActivityNotice(w, "app host port 8080 -> 8082")
				return nil
			})
			if err != nil || !strings.Contains(out.String(), "8080 -> 8082") {
				t.Fatalf("hidden topology change: %v %s", err, out.String())
			}
		}
	}
}

func TestBug871ConvergenceUsesObservedReadiness(t *testing.T) {
	for _, state := range []string{"ready", "unverified", "failed"} {
		s := application.StatusResult{State: "running", Ready: true}
		s.AddObservation("workload/web", state, "probe")
		var output bytes.Buffer
		err := reportApplicationConvergence(cli.NewTerminal(context.Background(), &output, &output), s)
		if strings.Contains(output.String(), "READY") != (state == "ready") {
			t.Fatalf("false READY: %s", output.String())
		}
		if (err != nil) != (state == "failed") {
			t.Fatalf("wrong result: %v", err)
		}
		if observedApplicationState(s) == "ready" && !s.Ready {
			t.Fatal("false observed readiness")
		}
	}
}

func TestBug867ObservedResumeAndNoopRefresh(t *testing.T) {
	configureTestTarget(t)
	root := httpsAdoptionFixture(t, false)
	t.Chdir(root)
	m := application.Manifest{Version: application.CurrentVersion, ApplicationID: application.MustNewApplicationID(), Name: "demo", Environment: "dev", Workload: application.WorkloadConfig{Components: []string{"demo-app"}}}
	mustWriteWizardTestFile(t, application.RepositoryManifestName, m.YAML())
	r, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	record, err := recordPendingDeployment(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.DeploymentRecord = &record
	for _, observation := range []struct {
		state string
		ready bool
	}{{"stopped", false}, {"ready", true}, {"ready", true}, {"running", false}} {
		if err := recordObservedDeployment(r, observation.state, observation.ready); err != nil {
			t.Fatal(err)
		}
		got, err := deployment.LoadDeploymentRecord(r.DeploymentIdentity)
		if err != nil {
			t.Fatal(err)
		}
		if got.Observed.State != observation.state || got.Observed.Ready != observation.ready || got.Observed.VerifiedAt.IsZero() == observation.ready {
			t.Fatalf("stale observation: %+v", got.Observed)
		}
	}
}

func TestBug866WorkloadAccessEnvironmentIncludesNativeBindings(t *testing.T) {
	m := application.WithWorkloadComponents(application.New("bindings", "dev", true, true, false), "app")
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication))
	t.Setenv(application.ProviderScopeEnv(capability.ProviderValkey), string(capability.ScopeApplication))
	store := application.Store{Root: t.TempDir()}
	files, err := application.EnsureRuntime(context.Background(), serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatal(err)
	}
	r := resolvedApplication{Manifest: m, Store: store}
	env, err := repositoryWorkloadEnvironment(context.Background(), r, files)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"DATABASE_URL", "DATABASE_CA_FILE", "REDIS_URL", "REDIS_CA_FILE"} {
		if env[key] == "" {
			t.Fatalf("missing native binding %s", key)
		}
	}
	if strings.Contains(env["DATABASE_URL"], "127.0.0.1") {
		t.Fatal("host contract used for workload")
	}
	if err := os.Remove(files.Env); err != nil {
		t.Fatal(err)
	}
	if _, err := repositoryWorkloadEnvironment(context.Background(), r, files); err == nil {
		t.Fatal("missing runtime bindings hidden")
	}
}
