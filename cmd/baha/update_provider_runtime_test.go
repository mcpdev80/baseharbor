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
	runCoreProviderVersionsRuntimeAcceptance(t, false)
}

func TestCoreHAOpenBaoVersionsRuntimeAcceptance(t *testing.T) {
	if os.Getenv("BASEHARBOR_HA_OPENBAO_UPGRADE_ACCEPTANCE") != "1" {
		t.Skip("requires isolated rootless HA OpenBao upgrade acceptance")
	}
	runCoreProviderVersionsRuntimeAcceptance(t, true)
}

func runCoreProviderVersionsRuntimeAcceptance(t *testing.T, ha bool) {
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
	opts := runtimeUpOptions{Yes: true, HA: ha, ControlPlaneOnly: true, RecoveryFile: filepath.Join(t.TempDir(), "recovery.json")}
	var files bhruntime.Files
	var output runtimeAcceptanceOutput
	state, err := coreinstallation.Run(ctx, root, coreinstallation.Spec{Target: target.Name, Runtime: engine, MachineRole: coreinstallation.Development, HA: ha}, coreinstallation.Steps{
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
			selected := append([]string(nil), files.OpenBaoMembers()...)
			images := map[string]string{}
			for _, member := range files.OpenBaoMembers() {
				images[member] = providerBaselineBao
			}
			if !ha {
				selected = append(selected, "postgres-member-1")
				images["postgres-member-1"] = providerBaselineSQL
			}
			if err := rt.StopProjectFilesSelected(ctx, files.Project, filepath.Dir(files.Compose), env, selected, files.Compose); err != nil {
				return err
			}
			if err := setProviderBaseline(files.Compose, images); err != nil {
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
			if ha {
				return identityprovider.EnsureCoreIdentity(ctx, rt, platformopenbao.NewServiceIssuer(rt, files), dataDir, target.Name, s.ID, true)
			}
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
		// Registered applications use their owned shared backend, not Core's
		// private SQL database. Authenticate with the actual binding and CA.
		shared := application.SharedBackendFilesAt(dataDir, target.Name, manifest.Environment)
		ca, err := os.ReadFile(appBinding.CertificatesPath)
		if err != nil {
			t.Fatal("application SQL trust unavailable")
		}
		const script = "IFS= read -r PGPASSWORD || exit 1\nexport PGPASSWORD PGSSLMODE=verify-full PGCONNECT_TIMEOUT=5\ntrust=$(mktemp /tmp/provider-acceptance-ca.XXXXXX) || exit 1\ntrap 'rm -f \"$trust\"' EXIT\ncat >\"$trust\"\nexport PGSSLROOTCERT=\"$trust\"\npsql -h postgres-access -U \"$1\" -d \"$2\" -Atqc \"$3\" -v ON_ERROR_STOP=1"
		input := append([]byte(appBinding.Password+"\n"), ca...)
		result, err := rt.ExecProjectInput(ctx, shared.Project, shared.Compose, shared.Env, input, "shared-postgres-dev", "sh", "-ec", script, "--", appBinding.Username, appBinding.Database, sql)
		if err != nil {
			t.Fatal("registered application SQL binding verification failed")
		}
		return strings.TrimSpace(result)
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
	// Finish productive bootstrap/application authoring before selecting the
	// previous pinned image baseline. Trust initialization can reconcile Core
	// images; it must not erase the real delta immediately before update.
	selected := append([]string(nil), files.OpenBaoMembers()...)
	images := map[string]string{}
	for _, member := range files.OpenBaoMembers() {
		images[member] = providerBaselineBao
	}
	if !ha {
		selected = append(selected, "postgres-member-1")
		images["postgres-member-1"] = providerBaselineSQL
	}
	environment, err := bhruntime.RuntimeEnvironment(files)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.StopProjectFilesSelected(ctx, files.Project, filepath.Dir(files.Compose), environment, selected, files.Compose); err != nil {
		t.Fatal(err)
	}
	if err := setProviderBaseline(files.Compose, images); err != nil {
		t.Fatal(err)
	}
	if err := rt.UpProjectFilesSelectedForceRecreateNoBuild(ctx, files.Project, filepath.Dir(files.Compose), environment, selected, files.Compose); err != nil {
		t.Fatal(err)
	}
	if err := waitForControlPlanePostgresMemberReady(ctx, rt, files, "postgres-member-1"); err != nil {
		t.Fatal(err)
	}
	if err := waitForOpenBaoExecReady(ctx, rt, files); err != nil {
		t.Fatal(err)
	}
	if err := platformopenbao.Unseal(ctx, rt, files, opts.RecoveryFile); err != nil {
		t.Fatal(err)
	}
	plan, err := inspectCoreRuntimePlan(ctx, "v0.4.24", state, rt)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []coreupdate.ProviderKind{coreupdate.SQL, coreupdate.Secrets, coreupdate.Identity} {
		d, err := providerDelta(plan, kind)
		if ha && kind != coreupdate.Secrets {
			if err != nil || d.Classification != coreupdate.NoChange {
				t.Fatalf("HA gate must only upgrade OpenBao: %s %+v %v", kind, d, err)
			}
			continue
		}
		if err != nil || d.Classification == coreupdate.NoChange || d.Classification == coreupdate.Unsupported {
			t.Fatalf("missing genuine %s version delta: %+v %v", kind, d, err)
		}
		t.Logf("Provider version transition: %s %s -> %s, source digest=%s target digest=%s", kind, d.Installed.Version, d.Desired.Version, d.Installed.Digest, d.Desired.Digest)
	}
	checkApplication()
	if ha {
		runHAOpenBaoUpgradeRecovery(t, ctx, ops, plan, engine, checkApplication, func() {
			if err := platformopenbao.SetApplicationSecret(ctx, rt, files, appIdentity, appCredentials, "UPGRADE_MARKER", []byte("post-backup-value")); err != nil {
				t.Fatal(err)
			}
		})
		return
	}
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
	// Exercise Single PostgreSQL failure recovery through the same native
	// recovery hook used by the productive mixed-provider transaction. Its
	// verified archive and Compose checkpoint were captured by that command.
	sqlDelta, err := providerDelta(plan, coreupdate.SQL)
	if err != nil {
		t.Fatal(err)
	}
	stateRoot, err := targetRuntimeStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	journalDir, err := coreUpdateJournal(stateRoot, "v0.4.24", false)
	if err != nil {
		t.Fatal(err)
	}
	volume, err := coreupdate.ResolveOwnedServiceVolume(files.Compose, "postgres-member-1", files.Project)
	if err != nil {
		t.Fatal(err)
	}
	assets := coreupdate.NativeProviderAssets{
		Recovery: coreupdate.VolumeRecovery{Runtime: rt, Directory: filepath.Join(journalDir, "backups"), Project: files.Project, Volume: volume, VerifyQuiesced: ops.verifyQuiesced},
		Compose:  coreupdate.ComposeCheckpoint{Path: files.Compose, Directory: filepath.Join(journalDir, "compose-backups")},
	}
	providerFixtureSQL(t, ctx, ops, false, "UPDATE public.baseharbor_upgrade_marker SET value='after-backup'")
	if err := ops.stopSelected(ctx, files, "postgres-member-1"); err != nil {
		t.Fatal(err)
	}
	if err := ops.VerifySemantics(ctx, sqlDelta); err == nil {
		t.Fatal("stopped PostgreSQL escaped verification")
	}
	if err := assets.Recover(ctx, sqlDelta, ops); err != nil {
		t.Fatalf("productive Single PostgreSQL original-volume/image recovery: %v", err)
	}
	if err := ops.verifyImage(ctx, sqlDelta, false); err != nil {
		t.Fatal(err)
	}
	if version := providerFixtureSQL(t, ctx, ops, false, "SHOW server_version_num"); version != beforeSQL {
		t.Fatal("PostgreSQL recovery did not restore original actual version")
	}
	if got := providerFixtureSQL(t, ctx, ops, false, "SELECT value FROM public.baseharbor_upgrade_marker"); got != "before-upgrade" {
		t.Fatal("PostgreSQL recovery did not rewind post-backup transactions")
	}
	checkApplication()
	checkIdentity()
	t.Log("Single PostgreSQL failure recovery restored verified original volume, image/version and backup-time data; native backing roles and application access retained")
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
		if c.Project != identity.Project || !strings.HasPrefix(c.Service, "keycloak-") {
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

// Only the changed HA OpenBao adapter is qualified here. SQL/DCS cutover
// evidence remains attached to its prior green run and is not repeated.
func runHAOpenBaoUpgradeRecovery(t *testing.T, ctx context.Context, ops *coreNativeRuntimeOps, plan coreupdate.Plan, engine string, checkApplication, changeSecret func()) {
	t.Helper()
	if len(ops.core.OpenBaoMembers()) != 3 {
		t.Fatal("HA gate requires three OpenBao members")
	}
	d, err := providerDelta(plan, coreupdate.Secrets)
	if err != nil {
		t.Fatal(err)
	}
	recoveryDir := filepath.Join(t.TempDir(), "ha-openbao-recovery")
	ops.receiptPath = filepath.Join(recoveryDir, "receipts.json")
	bound, err := ops.buildBoundProviderTransaction(ctx, plan, recoveryDir, engine)
	if err != nil {
		t.Fatal(err)
	}
	hooks := bound.Hooks()
	if err := hooks.RecoveryPoint(ctx, d); err != nil {
		t.Fatalf("HA OpenBao verified SQL backup: %v", err)
	}
	if err := reconcileNativeCoreProviders(ctx, "v0.4.24"); err != nil {
		t.Fatalf("productive HA OpenBao rolling upgrade: %v", err)
	}
	for _, member := range ops.core.OpenBaoMembers() {
		if err := ops.waitOpenBaoRollingMember(ctx, member, d.Desired.Version, d.Desired.Digest); err != nil {
			t.Fatal(err)
		}
	}
	checkApplication()
	t.Log("All three HA OpenBao members upgraded to pinned image, initialized and unsealed; real application SQL/AppRole/KV access preserved")
	changeSecret()
	if err := ops.stopSelected(ctx, ops.core, ops.core.OpenBaoMembers()...); err != nil {
		t.Fatal(err)
	}
	if err := hooks.Verify(ctx, d); err == nil {
		t.Fatal("stopped HA OpenBao escaped verification")
	}
	if err := hooks.Recover(ctx, d, "verify_failed"); err != nil {
		t.Fatalf("productive HA OpenBao SQL/config/image recovery: %v", err)
	}
	for _, member := range ops.core.OpenBaoMembers() {
		if err := ops.waitOpenBaoRollingMember(ctx, member, d.Installed.Version, d.Installed.Digest); err != nil {
			t.Fatal(err)
		}
	}
	checkApplication()
	if err := ops.VerifySemantics(ctx, d); err != nil {
		t.Fatal(err)
	}
	t.Log("HA OpenBao failure recovery restored original pinned image on all three members, SQL-backed KV state and real application access")
}
