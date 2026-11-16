package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestAppBackupGuidedRuntimeFixtureSelectsNativeTarget(t *testing.T) {
	for _, provider := range []string{"docker", "podman"} {
		t.Run(provider, func(t *testing.T) {
			t.Setenv("BASEHARBOR_TEST_RUNTIME", provider)
			t.Setenv("BASEHARBOR_TARGET", "foreign-selected-target")
			root := t.TempDir()
			configureAppBackupRuntimeTarget(t, filepath.Join(root, "cfg"), filepath.Join(root, "data"))
			target, err := effectiveTarget(context.Background())
			if err != nil || target.RuntimeProvider != provider || target.Name != "backup-recovery-ci" {
				t.Fatalf("fixture lost requested native Target: %+v %v", target, err)
			}
			cfg, err := deployment.LoadConfig()
			if err != nil || cfg.DefaultTarget != target.Name {
				t.Fatalf("default Target not persisted: %+v %v", cfg, err)
			}
			if _, err := os.Stat(filepath.Join(root, "cfg")); err != nil {
				t.Fatalf("isolated config directory was not created: %v", err)
			}
		})
	}
}

func TestAppBackupGuidedRuntimeFixtureRejectsInvalidRuntime(t *testing.T) {
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "not-a-native-runtime")
	// The contract parser must reject unsupported selection before a
	// real target or container orchestration is ever started.
	kind, err := backupFixtureRuntimeKind()
	if err == nil || kind != "" {
		t.Fatalf("unsupported runtime accepted: kind=%q err=%v", kind, err)
	}
}

func TestAppBackupCoreStateUsesSelectedTargetForNativeRuntimes(t *testing.T) {
	for _, provider := range []string{"docker", "podman"} {
		t.Run(provider, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("BASEHARBOR_TEST_RUNTIME", provider)
			t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(root, "unrelated-global-compose-state"))
			configureAppBackupRuntimeTarget(t, filepath.Join(root, "config"), filepath.Join(root, "data"))
			target, err := effectiveTarget(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			stateDir, err := targetRuntimeStateRoot(target)
			if err != nil {
				t.Fatal(err)
			}
			// Use the actual Core owner to generate its runtime input
			// definition; no Compose artifacts are fabricated for Podman.
			want, err := bhruntime.EnsureFilesForProjectAndResources(
				stateDir, targetRuntimeProjectName(target),
				bhruntime.SharedResourceProjectName(target.Name),
				bhruntime.Ports{Postgres: 15432, OpenBao: 18200}, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := bhruntime.ExistingFiles(""); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("isolated global lookup unexpectedly succeeds: %v", err)
			}
			got, err := existingTargetRuntimeFiles(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if got.Compose != want.Compose || got.Env != want.Env ||
				got.Project != want.Project || got.ResourceProject != want.ResourceProject {
				t.Fatalf("selected Core runtime differs: got %+v, want %+v", got, want)
			}
			if !strings.HasPrefix(got.Compose, stateDir+string(os.PathSeparator)) {
				t.Fatalf("Core files escaped selected Target: %q", got.Compose)
			}
			if err := os.Remove(want.Compose); err != nil {
				t.Fatal(err)
			}
			if _, err := existingTargetRuntimeFiles(context.Background()); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("missing selected Core input must fail closed: %v", err)
			}
		})
	}
}

func TestAppBackupOwnedCoreFixtureCleanupOnEveryExit(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		startErr error
		readyErr error
	}{
		{name: "initialized and authenticated"},
		{name: "partial startup error", startErr: errors.New("partial start")},
		{name: "readiness refuses initialized without manager", readyErr: errors.New("manager auth unavailable")},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			owned := filepath.Join(root, "owned")
			foreign := filepath.Join(root, "foreign")
			if err := os.WriteFile(owned, []byte("owned"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(foreign, []byte("foreign"), 0600); err != nil {
				t.Fatal(err)
			}
			calls := []string{}
			cancelled, cancel := context.WithCancel(context.Background())
			t.Cleanup(func() {
				if _, err := os.Stat(owned); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("owned state leaked: %v", err)
				}
				if _, err := os.Stat(foreign); err != nil {
					t.Errorf("foreign sentinel affected: %v", err)
				}
				want := []string{"start", "cleanup"}
				if scenario.startErr == nil {
					want = []string{"start", "ready", "cleanup"}
				}
				if fmt.Sprint(calls) != fmt.Sprint(want) {
					t.Errorf("fixture operations %v, want %v", calls, want)
				}
			})
			err := runOwnedBackupCoreFixture(t, cancelled,
				func(ctx context.Context) error { calls = append(calls, "start"); cancel(); return scenario.startErr },
				func(ctx context.Context) error { calls = append(calls, "ready"); return scenario.readyErr },
				func(ctx context.Context) error {
					calls = append(calls, "cleanup")
					if ctx.Err() != nil {
						return fmt.Errorf("cleanup inherited cancelled context")
					}
					if _, ok := ctx.Deadline(); !ok {
						return fmt.Errorf("cleanup missing bounded deadline")
					}
					return os.Remove(owned)
				},
			)
			var expected error
			if scenario.startErr != nil {
				expected = scenario.startErr
			} else {
				expected = scenario.readyErr
			}
			if expected == nil {
				if err != nil {
					t.Fatalf("successful fixture returned error: %v", err)
				}
			} else if err == nil || !errors.Is(err, expected) {
				t.Fatalf("fixture lost original failure: got %v, want %v", err, expected)
			}
		})
	}
}

func TestAppBackupOwnedCoreCleanupContextIsBounded(t *testing.T) {
	var cleaned bool
	t.Run("registered before failing start", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		_ = runOwnedBackupCoreFixture(t, ctx, func(context.Context) error {
			return errors.New("start failed")
		}, func(context.Context) error { t.Error("unexpected ready"); return nil },
			func(cleanup context.Context) error {
				cleaned = true
				if cleanup.Err() != nil {
					return cleanup.Err()
				}
				if deadline, ok := cleanup.Deadline(); !ok || time.Until(deadline) > 3*time.Minute {
					return errors.New("cleanup context not bounded")
				}
				return nil
			})
	})
	if !cleaned {
		t.Fatal("cleanup not executed on partial-start failure")
	}
}

func TestAppBackupOwnedCoreCleanupAfterLaterAbort(t *testing.T) {
	root := t.TempDir()
	owned, foreign := filepath.Join(root, "owned"), filepath.Join(root, "foreign")
	for _, file := range []string{owned, foreign} {
		if err := os.WriteFile(file, []byte("sentinel"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	calls := []string{}
	t.Run("later abort", func(t *testing.T) {
		err := runOwnedBackupCoreFixture(t, context.Background(),
			func(context.Context) error { calls = append(calls, "start"); return nil },
			func(context.Context) error { calls = append(calls, "ready"); return nil },
			func(ctx context.Context) error {
				calls = append(calls, "cleanup")
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if _, ok := ctx.Deadline(); !ok {
					return errors.New("unbounded cleanup")
				}
				return os.Remove(owned)
			})
		if err != nil {
			t.Fatalf("unexpected ready error: %v", err)
		}
		// SkipNow exits after readiness and still executes the fixture's
		// registered cleanup, without failing the containing regression.
		t.SkipNow()
	})
	if got, want := fmt.Sprint(calls), fmt.Sprint([]string{"start", "ready", "cleanup"}); got != want {
		t.Fatalf("cleanup order = %s, want %s", got, want)
	}
	if _, err := os.Stat(owned); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned state remained after abort: %v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("unrelated sentinel removed: %v", err)
	}
}

// Each operation first reaches its real fail-closed authority without consent.
// Only that invocation's valid typed plan can authorize one retry; stale plans
// and unrelated failures are returned, never refreshed in an approval loop.
func runBackupFixtureWithConsent(ctx context.Context, operation string, run func(context.Context) error) error {
	if operation != "apply" && operation != "destroy" && operation != "restore" {
		return errors.New("unsupported backup fixture operation")
	}
	ctx = withProviderChangeConsent(machineNoninteractiveContext(ctx), nil)
	err := run(ctx)
	if err == nil {
		return nil
	}
	ack, consentErr := backupFixtureConsent(operation, err)
	if consentErr != nil {
		return consentErr
	}
	return run(withProviderChangeConsent(ctx, ack))
}

func backupFixtureConsent(operation string, cause error) (*application.ProviderChangeConsent, error) {
	var failure *machine.Error
	if !errors.As(cause, &failure) || failure.Code != machine.ErrorApprovalRequired ||
		(failure.CauseCode != "impact-confirmation-required" && failure.CauseCode != "data-deletion-consent-required") {
		return nil, cause
	}
	var ack application.ProviderChangeConsent
	switch plan := failure.Details.(type) {
	case application.ProviderImpactPlan:
		if operation != "apply" && operation != "destroy" || plan.Destructive {
			return nil, cause
		}
		ack = application.ProviderChangeConsent{PlanID: plan.PlanID, First: true, Second: plan.ConfirmationsRequired == 2}
		if err := application.ValidateProviderChangeConsent(plan, ack); err != nil {
			return nil, err
		}
	case applicationDestructionPlan:
		// This opt-in recovery test explicitly authorizes replacement of its
		// own selected data, separately from retirement's retain-data consent.
		if operation != "restore" || plan.Scope.Owner.Owner != "application-restore" || !plan.Scope.Destructive {
			return nil, cause
		}
		ack = application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: plan.Scope.ConfirmationsRequired == 2, DeleteData: true}
		if err := application.ValidateProviderImpactScopeConsent(plan.Scope, ack); err != nil {
			return nil, err
		}
	default:
		return nil, cause
	}
	return &ack, nil
}

// Read the existing consumer binding on every phase, including after restore.
// Neither the physical provider project nor a bootstrap database role is guessed.
func backupFixtureSQL(ctx context.Context, resolved resolvedApplication, seed bool) (string, error) {
	_, binding, err := resolveAccessBinding(ctx, resolved.Store, resolved.Manifest.Name, "postgres", "default")
	if err != nil {
		return "", errors.New("backup fixture SQL consumer binding unavailable")
	}
	config, err := backupFixtureSQLConfig(binding)
	if err != nil {
		return "", err
	}
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return "", errors.New("backup fixture protected SQL consumer connection failed")
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()
	if seed {
		if _, err := conn.Exec(ctx, "CREATE TABLE recovery_probe(value text); INSERT INTO recovery_probe(value) VALUES ('survived');"); err != nil {
			return "", errors.New("backup fixture SQL consumer seed failed")
		}
	}
	var user, database, value string
	if err := conn.QueryRow(ctx, "SELECT current_user, current_database(), value FROM recovery_probe").Scan(&user, &database, &value); err != nil {
		return "", errors.New("backup fixture SQL consumer readback failed")
	}
	if user != binding.Username || database != binding.Database {
		return "", errors.New("backup fixture SQL consumer identity differs from the current binding")
	}
	return value, nil
}

func backupFixtureSQLConfig(binding application.ServiceBinding) (*pgx.ConnConfig, error) {
	uri, err := url.Parse(binding.URI)
	if err != nil || uri.Query().Get("sslmode") != "verify-ca" && uri.Query().Get("sslmode") != "verify-full" {
		return nil, errors.New("backup fixture SQL verified TLS required")
	}
	config, err := pgx.ParseConfig(binding.URI)
	if err != nil || config.TLSConfig == nil || config.Host != binding.Host ||
		fmt.Sprint(config.Port) != binding.Port || config.User != binding.Username ||
		config.Database != binding.Database || config.Password != binding.Password ||
		binding.Username == "" || binding.Database == "" || binding.Password == "" {
		return nil, errors.New("backup fixture SQL URI does not match its protected TLS consumer binding")
	}
	for _, fallback := range config.Fallbacks {
		if fallback.TLSConfig == nil {
			return nil, errors.New("backup fixture SQL plaintext fallback refused")
		}
	}
	return config, nil
}

func TestGuidedBackupFixtureRegressionSuite(t *testing.T) {
	for _, scenario := range []struct {
		name string
		run  func(*testing.T)
	}{
		{"native Target selection", TestAppBackupGuidedRuntimeFixtureSelectsNativeTarget},
		{"invalid runtime", TestAppBackupGuidedRuntimeFixtureRejectsInvalidRuntime},
		{"selected Core state", TestAppBackupCoreStateUsesSelectedTargetForNativeRuntimes},
		{"cleanup every exit", TestAppBackupOwnedCoreFixtureCleanupOnEveryExit},
		{"bounded cleanup", TestAppBackupOwnedCoreCleanupContextIsBounded},
		{"cleanup later abort", TestAppBackupOwnedCoreCleanupAfterLaterAbort},
		{"manifest contract", TestBackupFixtureManifestMatchesCapabilityContract},
		{"legacy manifest rejection", TestBackupFixtureManifestRejectsLegacyCapabilityKeys},
		{"resolved secret owner", TestGuidedBackupFixtureSecretsUseResolvedDeploymentScope},
	} {
		t.Run(scenario.name, scenario.run)
	}
}

func TestGuidedBackupFixtureSecretsUseResolvedDeploymentScope(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "podman")
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(root, "unrelated-platform-state"))
	configureAppBackupRuntimeTarget(t, filepath.Join(root, "config"), filepath.Join(root, "data"))
	target, err := effectiveTarget(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	calls := filepath.Join(root, "runtime-calls")
	t.Setenv("BACKUP_FIXTURE_RUNTIME_CALLS", calls)
	for name, script := range map[string]string{
		"podman-system-generator": "#!/bin/sh\nexit 0\n",
		"podman":                  "#!/bin/sh\nif [ \"$1\" = version ]; then exit 0; fi\nprintf '%s\\n' \"$*\" >> \"$BACKUP_FIXTURE_RUNTIME_CALLS\"\nexit 97\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	coreRoot, err := targetRuntimeStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	coreFiles, err := bhruntime.EnsureFilesForProjectAndResources(coreRoot,
		targetRuntimeProjectName(target), bhruntime.SharedResourceProjectName(target.Name),
		bhruntime.Ports{Postgres: 15432, OpenBao: 18200}, false)
	if err != nil {
		t.Fatal(err)
	}
	repository := filepath.Join(root, "repository")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repository)
	m := application.New("backup-cli", "dev", true, false, true)
	source := filepath.Join(repository, application.RepositoryManifestName)
	mustWriteInitializedAuthoredManifest(t, source, m)
	record := registerTestDeployment(t, target, m, repository, source)
	resolved, err := resolveApplication(t.Context(), application.DefaultStore(), nil, "backup")
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.FromRepository || resolved.Manifest.ApplicationID != m.ApplicationID ||
		resolved.DeploymentIdentity != record.Identity || resolved.Store.Namespace != target.Name ||
		resolved.Store.Root != filepath.Join(resolved.DeploymentStateRoot, "state") {
		t.Fatal("repository resolution lost its actual application UUID, Target or deployment owner")
	}
	if _, _, err := resolved.Store.Load(m.Name); err == nil {
		t.Fatal("fixture must exercise a canonical deployment without a legacy name-store entry")
	}
	files := application.RuntimeFilesFor(resolved.Store, m)
	if err := os.MkdirAll(files.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{files.Compose, files.Env} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	service, err := resolvedApplicationSecretService(t.Context(), resolved)
	if err != nil {
		t.Fatal("actual resolved secret owner rejected the repository deployment", err)
	}
	if err := service.Set(t.Context(), "other-application", "API_TOKEN", []byte("fixture-only")); err == nil ||
		!strings.Contains(err.Error(), "does not match resolved secret scope") {
		t.Fatal("another application escaped the resolved secret scope")
	}
	for _, operation := range []string{"set", "get"} {
		if operation == "set" {
			err = service.Set(t.Context(), m.Name, "API_TOKEN", []byte("fixture-only"))
		} else {
			_, err = service.Get(t.Context(), m.Name, "API_TOKEN")
		}
		if err == nil || strings.Contains(err.Error(), "not found") {
			t.Fatalf("%s did not reach the selected runtime from the resolved deployment: %v", operation, err)
		}
	}
	data, err := os.ReadFile(calls)
	if err != nil || !strings.Contains(string(data), coreFiles.Project) ||
		strings.Contains(string(data), "unrelated-platform-state") {
		t.Fatalf("secret operations did not use the actual selected Core runtime: %v", err)
	}
}

func TestGuidedBackupFixtureConsentIsFreshAndDoesNotRetryStalePlans(t *testing.T) {
	t.Setenv("BASEHARBOR_PROVIDER_CHANGE_CONSENT", "")
	for _, operation := range []string{"apply", "destroy"} {
		t.Run(operation, func(t *testing.T) {
			plan := application.ProviderImpactPlan{
				Version:               application.ProviderImpactVersion,
				PlanID:                operation + "-current",
				Target:                "isolated-fixture",
				ConfirmationsRequired: 2,
			}
			for _, stale := range []bool{false, true} {
				calls := 0
				err := runBackupFixtureWithConsent(t.Context(), operation, func(ctx context.Context) error {
					calls++
					ack, err := invocationProviderChangeConsent(ctx)
					if err != nil {
						return err
					}
					if calls == 1 {
						if ack != nil {
							t.Fatal("initial operation inherited consent")
						}
						return providerImpactConsentError(plan, &application.ProviderChangeError{Code: "impact-confirmation-required"})
					}
					if ack == nil || ack.PlanID != plan.PlanID || ack.DeleteData {
						t.Fatal("wrong operation plan or implicit deletion consent")
					}
					current := plan
					if stale {
						current.PlanID = "changed-after-preview"
					}
					return providerImpactConsentError(current, application.ValidateProviderChangeConsent(current, *ack))
				})
				if calls != 2 || (err != nil) != stale {
					t.Fatalf("calls=%d error=%v stale=%t", calls, err, stale)
				}
				if stale {
					var change *application.ProviderChangeError
					if !errors.As(err, &change) || change.Code != "stale-impact-consent" {
						t.Fatal("stale cause lost")
					}
				}
			}
		})
	}
}

func TestGuidedBackupFixtureRestoreRequiresItsOwnDestructiveScope(t *testing.T) {
	r := retainedNativeBackendFixture(t)
	data := applicationRestoreData{manifest: r.Manifest, archiveRevision: strings.Repeat("a", 64)}
	plan, err := buildApplicationRestorePlan(t.Context(), r, data)
	if err != nil {
		t.Fatal(err)
	}
	cause := applicationDestructionPlanError(plan, &application.ProviderChangeError{Code: "impact-confirmation-required"})
	ack, err := backupFixtureConsent("restore", cause)
	if err != nil || ack == nil || !ack.First || !ack.Second || !ack.DeleteData || ack.PlanID != plan.Scope.PlanID {
		t.Fatalf("explicit fixture restore scope missing: %v", err)
	}
	if err := authorizeApplicationRestore(withProviderChangeConsent(t.Context(), ack), r, data, io.Discard); err != nil {
		t.Fatal("actual restore authority rejected current explicit consent", err)
	}
	data.archiveRevision = strings.Repeat("b", 64)
	if err := authorizeApplicationRestore(withProviderChangeConsent(t.Context(), ack), r, data, io.Discard); err == nil {
		t.Fatal("changed archive reused restore approval")
	}
	for _, operation := range []string{"apply", "destroy"} {
		if got, err := backupFixtureConsent(operation, cause); err == nil || got != nil {
			t.Fatal("another operation inherited destructive restore approval")
		}
	}
	plan.Scope.PlanID = "tampered"
	if got, err := backupFixtureConsent("restore", applicationDestructionPlanError(plan, &application.ProviderChangeError{Code: "impact-confirmation-required"})); err == nil || got != nil {
		t.Fatal("invalid restore scope accepted")
	}
}

func TestGuidedBackupFixtureConsentPreservesUnrelatedFailures(t *testing.T) {
	t.Setenv("BASEHARBOR_PROVIDER_CHANGE_CONSENT", "")
	want := errors.New("unrelated fixture failure")
	calls := 0
	err := runBackupFixtureWithConsent(t.Context(), "apply", func(context.Context) error {
		calls++
		return want
	})
	if calls != 1 || !errors.Is(err, want) {
		t.Fatal("unrelated failure was retried or replaced")
	}
	for _, plan := range []application.ProviderImpactPlan{
		{Version: application.ProviderImpactVersion, PlanID: "destructive", ConfirmationsRequired: 2, Destructive: true},
		{Version: application.ProviderImpactVersion, PlanID: "", ConfirmationsRequired: 2},
		{Version: application.ProviderImpactVersion, PlanID: "invalid-stages", ConfirmationsRequired: 3},
	} {
		cause := providerImpactConsentError(plan, &application.ProviderChangeError{Code: "impact-confirmation-required"})
		if ack, err := backupFixtureConsent("apply", cause); err == nil || ack != nil {
			t.Fatal("invalid or destructive apply implicitly acknowledged")
		}
	}
	cause := &machine.Error{Code: machine.ErrorApprovalRequired, CauseCode: "impact-confirmation-required", Details: map[string]string{"plan_id": "untyped"}}
	if ack, err := backupFixtureConsent("apply", cause); err == nil || ack != nil {
		t.Fatal("untyped details accepted")
	}
}

func TestGuidedBackupFixtureSQLConfigUsesActualConsumerAndVerifiedTLS(t *testing.T) {
	binding := application.ServiceBinding{
		Host:     "sql.example.invalid",
		Port:     "6432",
		Username: "application_role",
		Database: "actual_database",
		Password: "private-fixture-sentinel",
		URI:      "postgresql://application_role:private-fixture-sentinel@sql.example.invalid:6432/actual_database?sslmode=verify-full",
	}
	cfg, err := backupFixtureSQLConfig(binding)
	if err != nil || cfg == nil || cfg.TLSConfig == nil || cfg.TLSConfig.InsecureSkipVerify {
		t.Fatal("verified consumer configuration unavailable")
	}
	for _, mutate := range []func(*application.ServiceBinding){
		func(b *application.ServiceBinding) { b.Username = "another_role" },
		func(b *application.ServiceBinding) { b.Database = "another_database" },
		func(b *application.ServiceBinding) { b.Password = "another_password" },
		func(b *application.ServiceBinding) { b.Host = "another.example.invalid" },
		func(b *application.ServiceBinding) { b.Port = "5432" },
		func(b *application.ServiceBinding) { b.URI = strings.ReplaceAll(b.URI, "verify-full", "disable") },
		func(b *application.ServiceBinding) { b.URI = strings.ReplaceAll(b.URI, "verify-full", "prefer") },
		func(b *application.ServiceBinding) { b.URI = strings.ReplaceAll(b.URI, "verify-full", "require") },
		func(b *application.ServiceBinding) { b.URI = "invalid%private-fixture-sentinel" },
	} {
		bad := binding
		mutate(&bad)
		got, err := backupFixtureSQLConfig(bad)
		if got != nil || err == nil || strings.Contains(err.Error(), binding.Password) {
			t.Fatal("invalid SQL binding accepted or credential disclosed")
		}
	}
}
