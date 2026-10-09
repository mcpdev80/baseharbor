package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"go.yaml.in/yaml/v3"
)

const providerBaselineSQL = "docker.io/library/postgres:18.0-alpine@sha256:48c8ad3a7284b82be4482a52076d47d879fd6fb084a1cbfccbd551f9331b0e40"
const providerBaselineBao = "docker.io/openbao/openbao:2.7.0@sha256:71156a1c6623a5fa3f5e61b0c6a8ead0faf0df29a778339188443551995d1315"
const providerBaselineIdentity = "quay.io/keycloak/keycloak:26.7.5@sha256:37dbaf6f0722c9ec246335f36e1ef8b2e6cb960f7c27e0d8c615121a3d475a85"

// This wrapper chooses the initial real Keycloak image before the production
// bootstrap starts it. Every operation delegates to the actual native engine.
// Provider update/backup/recovery never use the wrapper or mocked hooks.
type baselineIdentityRuntime struct{ bhruntime.RuntimeProvider }

func (r baselineIdentityRuntime) UpProject(ctx context.Context, project, compose, env string) error {
	if err := setProviderBaseline(compose, map[string]string{"keycloak-1": providerBaselineIdentity}); err != nil {
		return err
	}
	return r.RuntimeProvider.UpProject(ctx, project, compose, env)
}
func (r baselineIdentityRuntime) UpProjectFilesSelected(ctx context.Context, project, dir string, environment map[string]string, services []string, files ...string) error {
	if len(files) != 1 {
		return fmt.Errorf("baseline requires one owned Compose")
	}
	if err := setProviderBaseline(files[0], map[string]string{"keycloak-1": providerBaselineIdentity}); err != nil {
		return err
	}
	return r.RuntimeProvider.UpProjectFilesSelected(ctx, project, dir, environment, services, files...)
}
func setProviderBaseline(path string, images map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	services, ok := doc["services"].(map[string]any)
	if !ok {
		return fmt.Errorf("missing baseline services")
	}
	for service, image := range images {
		entry, ok := services[service].(map[string]any)
		if !ok {
			return fmt.Errorf("missing baseline service %s", service)
		}
		entry["image"] = image
	}
	data, err = yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func TestCoreProviderVersionsRuntimeAcceptance(t *testing.T) {
	if os.Getenv("BASEHARBOR_PROVIDER_UPGRADE_ACCEPTANCE") != "1" {
		t.Skip("requires isolated rootless Docker/Podman provider acceptance")
	}
	engine := os.Getenv("BASEHARBOR_TEST_RUNTIME")
	if engine == "" {
		engine = "docker"
	}
	format := "{{json .SecurityOptions}}"
	if engine == "podman" {
		format = "{{.Host.Security.Rootless}}"
	}
	probe, err := exec.Command(engine, "info", "--format", format).Output()
	if err != nil || (engine == "docker" && !strings.Contains(string(probe), "rootless")) || (engine == "podman" && strings.TrimSpace(string(probe)) != "true") {
		t.Fatal("verified rootless runtime required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Minute)
	defer cancel()
	target := configureTestTarget(t)
	if engine == "podman" {
		cfg, err := deployment.LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		def := cfg.Targets[target.Name]
		def.Runtime.Provider = engine
		cfg.Targets[target.Name] = def
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		target, err = cfg.ResolveTarget("", "")
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(t.TempDir())
	rt, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := rt.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range inventory {
		if c.Project == targetRuntimeProjectName(target) || c.Project == bhruntime.SharedProjectName(target.Name+"-core") {
			t.Fatal("existing owned project refused")
		}
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 3*time.Minute)
		defer stop()
		if t.Failed() {
			logCoreBootstrapFailure(t, rt, target.Name, engine)
			logProviderBaselineDiagnostics(t, rt, target.Name, engine)
		}
		_ = runWithIO(cleanup, []string{"app", "destroy", "--yes"}, io.Discard, io.Discard)
		if err := runtimeDestroy(cleanup, []string{"--yes"}, io.Discard); err != nil {
			t.Errorf("owned fixture cleanup: %v", err)
		}
	}()
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	dataDir, err := targetDataRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	opts := runtimeUpOptions{Yes: true, ControlPlaneOnly: true, RecoveryFile: filepath.Join(t.TempDir(), "recovery.json")}
	var files bhruntime.Files
	var output runtimeAcceptanceOutput
	state, err := coreinstallation.Run(ctx, root, coreinstallation.Spec{Target: target.Name, Runtime: engine, MachineRole: coreinstallation.Development}, coreinstallation.Steps{
		Preflight: func(context.Context, coreinstallation.State) error { return nil },
		SQL: func(ctx context.Context, _ coreinstallation.State) error {
			if err := runtimeUpGuidedProviders(ctx, strings.NewReader(""), &output, opts); err != nil {
				return err
			}
			var err error
			files, err = existingTargetRuntimeFiles(ctx)
			if err != nil {
				return err
			}
			env, err := bhruntime.RuntimeEnvironment(files)
			if err != nil {
				return err
			}
			selected := []string{"postgres-member-1", "openbao-member-1"}
			if err := rt.StopProjectFilesSelected(ctx, files.Project, filepath.Dir(files.Compose), env, selected, files.Compose); err != nil {
				return err
			}
			if err := setProviderBaseline(files.Compose, map[string]string{"postgres-member-1": providerBaselineSQL, "openbao-member-1": providerBaselineBao}); err != nil {
				return err
			}
			if err := rt.UpProjectFilesSelectedForceRecreateNoBuild(ctx, files.Project, filepath.Dir(files.Compose), env, selected, files.Compose); err != nil {
				return err
			}
			if err := waitForControlPlanePostgresMemberReady(ctx, rt, files, "postgres-member-1"); err != nil {
				return err
			}
			return waitForOpenBaoExecReady(ctx, rt, files)
		},
		Secrets: func(ctx context.Context, _ coreinstallation.State) error {
			return ensureRepositoryOpenBaoReady(ctx, strings.NewReader(""), &output, io.Discard, opts)
		},
		Identity: func(ctx context.Context, s coreinstallation.State) (string, error) {
			return identityprovider.EnsureCoreIdentity(ctx, baselineIdentityRuntime{rt}, platformopenbao.NewServiceIssuer(rt, files), dataDir, target.Name, s.ID)
		},
		Verify: func(ctx context.Context, s coreinstallation.State) error {
			return identityprovider.VerifyCoreOperatorTokenFlow(ctx, dataDir, target.Name, s.ID, s.IdentityIssuer)
		},
	})
	if err != nil {
		t.Fatalf("real provider baseline bootstrap: %v", err)
	}
	identity, err := identityprovider.ExistingCoreRuntimeFiles(dataDir, target.Name)
	if err != nil {
		t.Fatal(err)
	}
	ops := &coreNativeRuntimeOps{runtime: rt, core: files, identity: identity, dataDir: dataDir, target: target.Name, installation: state.ID, issuer: state.IdentityIssuer, release: "v0.4.24"}
	plan, err := inspectCoreRuntimePlan(ctx, "v0.4.24", state, rt)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []coreupdate.ProviderKind{coreupdate.SQL, coreupdate.Secrets, coreupdate.Identity} {
		d, err := providerDelta(plan, kind)
		if err != nil || d.Classification == coreupdate.NoChange || d.Classification == coreupdate.Unsupported {
			t.Fatalf("missing genuine %s version delta: %+v %v", kind, d, err)
		}
		t.Logf("Provider version transition: %s %s -> %s, source digest=%s target digest=%s", kind, d.Installed.Version, d.Desired.Version, d.Installed.Digest, d.Desired.Digest)
	}
	// A real registered SQL/Secrets application makes semantic checks non-vacuous.
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeShared))
	manifest := application.New("provider-upgrade-acceptance", "dev", true, false, true)
	if err := os.WriteFile(application.RepositoryManifestName, []byte(manifest.YAML()), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runWithIO(ctx, []string{"app", "apply"}, &output, &output); err != nil {
		t.Fatalf("baseline application: %v", err)
	}
	if err := ops.verifyOwnedApplicationSecretScopes(ctx); err != nil {
		t.Fatal(err)
	}
	resolved, appBinding, err := resolveAccessBinding(ctx, application.DefaultStore(), "", "postgres", "default")
	if err != nil {
		t.Fatal(err)
	}
	appFiles, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	appIdentity := platformopenbao.ApplicationIdentity{Name: manifest.Name, Environment: manifest.Environment}
	appCredentials := platformopenbao.ApplicationCredentialsPath(appFiles.Dir)
	secretValue := []byte("provider-acceptance-before-upgrade")
	if err := platformopenbao.SetApplicationSecret(ctx, rt, files, appIdentity, appCredentials, "UPGRADE_MARKER", secretValue); err != nil {
		t.Fatal(err)
	}
	applicationSQL := func(sql string) string {
		return providerFixtureSQLCredentials(t, ctx, ops.core, rt, "postgres-admin", "postgres", "/run/baseharbor/postgres-ca/ca.pem", appBinding.Username, appBinding.Password, appBinding.Database, sql)
	}
	applicationSQL("CREATE TABLE public.provider_upgrade_marker(value text NOT NULL); INSERT INTO public.provider_upgrade_marker VALUES ('application-retained')")
	checkApplication := func() {
		if applicationSQL("SELECT value FROM public.provider_upgrade_marker") != "application-retained" {
			t.Fatal("application SQL data or credentials lost")
		}
		if applicationSQL("SELECT CASE WHEN NOT rolsuper AND NOT rolcreaterole AND NOT rolcreatedb THEN 'least-privilege' ELSE 'unsafe' END FROM pg_roles WHERE rolname=current_user") != "least-privilege" {
			t.Fatal("application SQL role gained administrative privileges")
		}
		value, err := platformopenbao.GetApplicationSecret(ctx, rt, files, appIdentity, appCredentials, "UPGRADE_MARKER")
		if err != nil || !bytes.Equal(value, secretValue) {
			t.Fatal("application AppRole cannot read preserved secret")
		}
	}
	checkApplication()
	checkIdentity, changeIdentityUser := providerFixtureIdentity(t, ctx, ops)
	beforeSQL := providerFixtureSQL(t, ctx, ops, false, "SHOW server_version_num")
	providerFixtureSQL(t, ctx, ops, false, "CREATE TABLE public.baseharbor_upgrade_marker(value text NOT NULL); INSERT INTO public.baseharbor_upgrade_marker VALUES ('before-upgrade')")
	providerFixtureSQL(t, ctx, ops, true, "CREATE TABLE public.baseharbor_upgrade_marker(value text NOT NULL); INSERT INTO public.baseharbor_upgrade_marker VALUES ('before-upgrade')")
	// Save separate native adapter backups for controlled real failure/recovery.
	recoveryDir := filepath.Join(t.TempDir(), "provider-recovery")
	ops.receiptPath = filepath.Join(recoveryDir, "receipts.json")
	bound, err := ops.buildBoundProviderTransaction(ctx, plan, recoveryDir, engine)
	if err != nil {
		t.Fatal(err)
	}
	hooks := bound.Hooks()
	for _, kind := range []coreupdate.ProviderKind{coreupdate.Secrets, coreupdate.Identity} {
		d, _ := providerDelta(plan, kind)
		if err := hooks.RecoveryPoint(ctx, d); err != nil {
			t.Fatalf("%s verified native SQL backup: %v", kind, err)
		}
	}
	// The full productive command path owns ordering, snapshots and durable journals.
	if err := reconcileNativeCoreProviders(ctx, "v0.4.24"); err != nil {
		t.Fatalf("productive provider reconciliation: %v", err)
	}
	checkIdentity()
	afterSQL := providerFixtureSQL(t, ctx, ops, false, "SHOW server_version_num")
	if beforeSQL == afterSQL {
		t.Fatal("PostgreSQL image change did not change actual server version")
	}
	for _, isIdentity := range []bool{false, true} {
		if got := providerFixtureSQL(t, ctx, ops, isIdentity, "SELECT value FROM public.baseharbor_upgrade_marker"); got != "before-upgrade" {
			t.Fatal("upgrade lost SQL marker")
		}
	}
	if err := ops.verifyOwnedApplicationSecretScopes(ctx); err != nil {
		t.Fatal(err)
	}
	checkApplication()
	t.Logf("PostgreSQL actual version %s -> %s; registered SQL/Secrets application retained", beforeSQL, afterSQL)
	// Fail real services, then use the same production Recover hooks, not fake Ops.
	for _, kind := range []coreupdate.ProviderKind{coreupdate.Identity, coreupdate.Secrets} {
		d, _ := providerDelta(plan, kind)
		if kind == coreupdate.Identity {
			changeIdentityUser()
			providerFixtureSQL(t, ctx, ops, true, "UPDATE public.baseharbor_upgrade_marker SET value='after-backup'")
			if err := ops.stopSelected(ctx, bhruntime.Files{Project: identity.Project, Compose: identity.Compose, Env: identity.Env}, "keycloak-1"); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := platformopenbao.SetApplicationSecret(ctx, rt, files, appIdentity, appCredentials, "UPGRADE_MARKER", []byte("post-backup-value")); err != nil {
				t.Fatal(err)
			}
			if err := ops.stopSelected(ctx, files, files.OpenBaoMembers()...); err != nil {
				t.Fatal(err)
			}
		}
		if err := hooks.Verify(ctx, d); err == nil {
			t.Fatalf("%s real service failure escaped verification", kind)
		}
		if err := hooks.Recover(ctx, d, "verify_failed"); err != nil {
			t.Fatalf("%s productive SQL/config/image recovery: %v", kind, err)
		}
		if err := ops.verifyImage(ctx, d, false); err != nil {
			t.Fatal(err)
		}
		if err := ops.VerifySemantics(ctx, d); err != nil {
			t.Fatal(err)
		}
		t.Logf("Native %s failure and verified original SQL/image/semantic recovery passed", kind)
	}
	if got := providerFixtureSQL(t, ctx, ops, true, "SELECT value FROM public.baseharbor_upgrade_marker"); got != "before-upgrade" {
		t.Fatal("Keycloak recovery did not restore backup marker")
	}
	if err := ops.verifyOwnedApplicationSecretScopes(ctx); err != nil {
		t.Fatal(err)
	}
	checkApplication()
	checkIdentity()
	t.Log("Real provider upgrades and controlled failure recovery passed; no production resources used")
}

func logProviderBaselineDiagnostics(t *testing.T, rt bhruntime.RuntimeProvider, target, engine string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	inventory, err := rt.ListRuntimeContainers(ctx)
	if err != nil {
		return
	}
	backend := bhruntime.NewCLIBackend(engine)
	dataDir, err := targetDataRoot(mustEffectiveTestTarget(t, ctx))
	if err != nil {
		return
	}
	identity, err := identityprovider.ExistingCoreRuntimeFiles(dataDir, target)
	if err != nil {
		return
	}
	values, err := bhruntime.RuntimeEnvironment(bhruntime.Files{Env: identity.Env})
	if err != nil {
		return
	}
	for _, c := range inventory {
		if c.Project != identity.Project || (c.Service != "keycloak-access" && c.Service != "keycloak-1") {
			continue
		}
		logs, err := backend.DirectOutput(ctx, "logs", "--tail", "100", c.ID)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(logs, "\n") {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "\"request\"") || strings.Contains(lower, "token") || strings.Contains(lower, "password") {
				continue
			}
			if !strings.Contains(lower, "error") && !strings.Contains(lower, "warn") && !strings.Contains(lower, "tls") && !strings.Contains(lower, "started") {
				continue
			}
			// Exclude credential and request records from provider diagnostics;
			// protected environment values are still removed before publication.
			t.Logf("Provider bootstrap diagnostic %s: %s", c.Service, sanitizeWorkloadDiagnostic(line, values))
		}
	}
}

func providerFixtureSQL(t *testing.T, ctx context.Context, o *coreNativeRuntimeOps, identity bool, sql string) string {
	t.Helper()
	files := o.core
	client, host, ca, database := "postgres-admin", "postgres", "/run/baseharbor/postgres-ca/ca.pem", "postgres"
	credentials, err := bhruntime.LoadControlPlaneCredentials(o.core)
	if err != nil {
		t.Fatal(err)
	}
	user, password := credentials.PostgresUser, credentials.PostgresPassword
	if identity {
		files = bhruntime.Files{Project: o.identity.Project, Compose: o.identity.Compose, Env: o.identity.Env}
		client, host, ca = "keycloak-db-init", "keycloak-db", "/run/baseharbor/db-tls/ca.pem"
		values, err := bhruntime.RuntimeEnvironment(files)
		if err != nil {
			t.Fatal(err)
		}
		user, password, database = values["BASEHARBOR_KEYCLOAK_DB_USER"], values["BASEHARBOR_KEYCLOAK_DB_PASSWORD"], values["BASEHARBOR_KEYCLOAK_DB_NAME"]
	}
	return providerFixtureSQLCredentials(t, ctx, files, o.runtime, client, host, ca, user, password, database, sql)
}

func providerFixtureSQLCredentials(t *testing.T, ctx context.Context, files bhruntime.Files, rt bhruntime.RuntimeProvider, client, host, ca, user, password, database, sql string) string {
	t.Helper()
	values, err := bhruntime.RuntimeEnvironment(files)
	if err != nil {
		t.Fatal(err)
	}
	script := "IFS= read -r PGPASSWORD || exit 1\nexport PGPASSWORD PGSSLMODE=verify-full PGSSLROOTCERT=\"$1\" PGCONNECT_TIMEOUT=5\nexec psql -h \"$2\" -U \"$3\" -d \"$4\" -Atqc \"$5\" -v ON_ERROR_STOP=1"
	var result bytes.Buffer
	if err := rt.RunProjectFilesEnv(ctx, files.Project, filepath.Dir(files.Compose), values, strings.NewReader(password+"\n"), &result, io.Discard, []string{files.Compose}, "run", "--rm", "--no-deps", client, "sh", "-ec", script, "--", ca, host, user, database, sql); err != nil {
		t.Fatal("provider fixture SQL failed")
	}
	return strings.TrimSpace(result.String())
}
