package application

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

type postgresBackupCall struct {
	project, compose, env string
	service               string
	args                  []string
	input                 []byte
}

type fakePostgresBackupRuntime struct {
	dumps        map[string]string
	execErr      map[string]error
	restoreErr   map[string]error
	verifyResult map[string]string
	calls        []postgresBackupCall
}

func (f *fakePostgresBackupRuntime) ExecProject(_ context.Context, project, compose, env, service string, args ...string) (string, error) {
	f.calls = append(f.calls, postgresBackupCall{project: project, compose: compose, env: env, service: service, args: append([]string(nil), args...)})
	if err := f.execErr[service]; err != nil {
		return "", err
	}
	if strings.Contains(strings.Join(args, " "), "pg_dump") {
		return f.dumps[service], nil
	}
	if strings.Contains(strings.Join(args, " "), "psql") {
		if result, ok := f.verifyResult[service]; ok {
			return result, nil
		}
		return "1\n", nil
	}
	return "", nil
}

func (f *fakePostgresBackupRuntime) ExecProjectInput(_ context.Context, project, compose, env string, input []byte, service string, args ...string) (string, error) {
	f.calls = append(f.calls, postgresBackupCall{project: project, compose: compose, env: env, service: service, args: append([]string(nil), args...), input: append([]byte(nil), input...)})
	if err := f.restoreErr[service]; err != nil {
		return "", err
	}
	if strings.Contains(strings.Join(args, " "), "pg_dump") {
		return f.dumps[service], nil
	}
	return "", nil
}

func TestDumpPostgresInstancesUsesStableMultiInstanceIdentity(t *testing.T) {
	m := WithSQLInstances(New("mailflow", "dev", true, false, false), "analytics", "primary")
	runtime := &fakePostgresBackupRuntime{dumps: map[string]string{
		"postgres-analytics": "-- analytics dump\nCREATE TABLE analytics_probe(id integer);\n",
		"postgres-primary":   "-- primary dump\nCREATE TABLE primary_probe(id integer);\n",
	}}
	files := RuntimeFiles{Compose: "/runtime/compose.yaml", Env: "/runtime/runtime.env"}

	backups, err := DumpPostgresInstances(context.Background(), runtime, m, files)
	if err != nil {
		t.Fatalf("DumpPostgresInstances() error = %v", err)
	}
	if got, want := []string{backups[0].Instance, backups[1].Instance}, []string{"analytics", "primary"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("instances = %v, want %v", got, want)
	}
	if len(runtime.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(runtime.calls))
	}
	for _, call := range runtime.calls {
		joined := strings.Join(call.args, " ")
		if !strings.Contains(joined, "--clean --if-exists --no-owner --no-privileges --format=plain") {
			t.Fatalf("dump args = %q", joined)
		}
		if !strings.Contains(joined, `PGPASSWORD="$POSTGRES_PASSWORD"`) || !strings.Contains(joined, "PGSSLMODE=verify-ca") || strings.Contains(joined, "-U baseharbor") {
			t.Fatalf("dump must use retained protected credentials and TLS: %q", joined)
		}
	}
	if got := strings.Join(runtime.calls[0].args, " "); !strings.Contains(got, "PGHOST='postgres-analytics'") || !strings.Contains(got, `PGDATABASE="$POSTGRES_DB"`) {
		t.Fatalf("analytics database not selected: %q", got)
	}
	if got := strings.Join(runtime.calls[1].args, " "); !strings.Contains(got, "PGHOST='postgres-primary'") || !strings.Contains(got, `PGDATABASE="$POSTGRES_DB"`) {
		t.Fatalf("primary database not selected: %q", got)
	}
}

func TestRestorePostgresInstancesUsesStdinAndVerifiesEachInstance(t *testing.T) {
	m := WithSQLInstances(New("mailflow", "dev", true, false, false), "analytics", "primary")
	runtime := &fakePostgresBackupRuntime{verifyResult: map[string]string{
		"postgres-analytics": "1\n",
		"postgres-primary":   "1\n",
	}}
	files := RuntimeFiles{Compose: "/runtime/compose.yaml", Env: "/runtime/runtime.env"}
	backups := []PostgresBackup{
		{Instance: "primary", SQL: []byte("CREATE TABLE primary_probe(id integer);\n")},
		{Instance: "analytics", SQL: []byte("CREATE TABLE analytics_probe(id integer);\n")},
	}

	if err := RestorePostgresInstances(context.Background(), runtime, m, files, backups); err != nil {
		t.Fatalf("RestorePostgresInstances() error = %v", err)
	}
	if len(runtime.calls) != 4 {
		t.Fatalf("calls = %d, want 4", len(runtime.calls))
	}
	for i := 0; i < len(runtime.calls); i += 2 {
		restore := runtime.calls[i]
		verify := runtime.calls[i+1]
		if len(restore.input) == 0 {
			t.Fatalf("restore for %s did not receive SQL over stdin", restore.service)
		}
		joined := strings.Join(restore.args, " ")
		if !strings.Contains(joined, "--no-psqlrc -v ON_ERROR_STOP=1") {
			t.Fatalf("restore args = %q", joined)
		}
		if strings.Contains(joined, string(restore.input)) {
			t.Fatalf("restore SQL leaked into process arguments")
		}
		if got := strings.Join(verify.args, " "); !strings.Contains(got, "-tAc 'SELECT 1'") {
			t.Fatalf("verify args = %q", got)
		}
	}
}

func TestValidatePostgresBackupSetFailsClosed(t *testing.T) {
	m := WithSQLInstances(New("mailflow", "dev", true, false, false), "analytics", "primary")
	tests := []struct {
		name    string
		backups []PostgresBackup
		want    string
	}{
		{name: "missing", backups: []PostgresBackup{{Instance: "primary", SQL: []byte("SELECT 1;")}}, want: "instance count mismatch"},
		{name: "unexpected", backups: []PostgresBackup{{Instance: "primary", SQL: []byte("SELECT 1;")}, {Instance: "other", SQL: []byte("SELECT 1;")}}, want: "unexpected instance"},
		{name: "duplicate", backups: []PostgresBackup{{Instance: "primary", SQL: []byte("SELECT 1;")}, {Instance: "primary", SQL: []byte("SELECT 1;")}}, want: "duplicate instance"},
		{name: "empty", backups: []PostgresBackup{{Instance: "analytics", SQL: nil}, {Instance: "primary", SQL: []byte("SELECT 1;")}}, want: "is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePostgresBackupSet(m, tt.backups)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidatePostgresBackupSet() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestRestorePostgresInstancesStopsOnRestoreFailure(t *testing.T) {
	m := New("mailflow", "dev", true, false, false)
	runtime := &fakePostgresBackupRuntime{restoreErr: map[string]error{"postgres": errors.New("restore failed")}}
	err := RestorePostgresInstances(context.Background(), runtime, m, RuntimeFiles{}, []PostgresBackup{{Instance: "default", SQL: []byte("SELECT 1;")}})
	if err == nil || !strings.Contains(err.Error(), "restore postgres instance default") {
		t.Fatalf("RestorePostgresInstances() error = %v", err)
	}
	if len(runtime.calls) != 1 {
		t.Fatalf("calls after failed restore = %d, want 1", len(runtime.calls))
	}
}

func TestDumpSharedPostgresInstancesUsesOnlyRegisteredApplicationResources(t *testing.T) {
	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), "shared")
	m := WithSQLInstances(New("app-a", "dev", true, false, false), "default", "analytics")
	root := t.TempDir()
	shared := SharedBackendFilesAt(root, "baha", m.Environment)
	if err := os.MkdirAll(shared.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	adminRef, err := ensureSharedPostgresCredential(shared.Dir, "provider-admin", "")
	if err != nil {
		t.Fatal(err)
	}
	defaultRef, err := ensureSharedPostgresCredential(shared.Dir, sharedBackendApplicationKey(m), "default")
	if err != nil {
		t.Fatal(err)
	}
	analyticsRef, err := ensureSharedPostgresCredential(shared.Dir, sharedBackendApplicationKey(m), "analytics")
	if err != nil {
		t.Fatal(err)
	}
	state := sharedBackendState{
		Version: sharedBackendStateVersion, Environment: m.Environment, PostgresAdminCredential: adminRef, PostgresMembers: 1,
		Applications: map[string]sharedBackendAppState{
			sharedBackendApplicationKey(m): {
				Application: m.Name, Environment: m.Environment,
				SQL: map[string]sharedPostgresResource{
					"default":   {Database: "app_a_dev", Username: "baha_app_a_dev", CredentialReference: defaultRef},
					"analytics": {Database: "app_a_dev_analytics", Username: "baha_app_a_dev_analytics", CredentialReference: analyticsRef},
				},
			},
			"app-b/dev": {
				Application: "app-b", Environment: "dev",
				SQL: map[string]sharedPostgresResource{
					"default": {Database: "app_b_dev", Username: "baha_app_b_dev", CredentialReference: "credentials/postgres/app-b-dev-default.password"},
				},
			},
		},
	}
	if err := os.MkdirAll(filepath.Join(shared.Dir, "credentials", "postgres"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeOwnerOnlyFile(filepath.Join(shared.Dir, "credentials", "postgres", "app-b-dev-default.password"), []byte("app-b-secret\n")); err != nil {
		t.Fatal(err)
	}
	if err := writeSharedBackendState(shared.State, state); err != nil {
		t.Fatal(err)
	}

	runtime := &fakePostgresBackupRuntime{dumps: map[string]string{
		sharedPostgresService(m.Environment): "-- app-a dump\nSELECT 1;\n",
	}}
	backups, err := DumpPostgresInstancesAt(context.Background(), runtime, m, RuntimeFiles{}, root, "baha")
	if err != nil {
		t.Fatalf("DumpPostgresInstancesAt() error = %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("backups = %d, want 2", len(backups))
	}
	if len(runtime.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(runtime.calls))
	}
	for _, call := range runtime.calls {
		joined := strings.Join(call.args, " ")
		if strings.Contains(joined, "app_b_dev") || strings.Contains(joined, "baha_app_b_dev") {
			t.Fatalf("shared backup crossed application boundary: %q", joined)
		}
		if !strings.Contains(joined, "pg_dump --clean --if-exists --no-owner --no-privileges --format=plain") {
			t.Fatalf("shared dump args = %q", joined)
		}
		if !strings.Contains(joined, "--exclude-schema=metric_helpers") || strings.Contains(joined, " --schema=") {
			t.Fatalf("shared dump must preserve custom application schemas without restoring provider monitoring objects: %q", joined)
		}
		for _, providerObject := range []string{"schema=user_management", "extension=pg_stat_statements", "extension=pg_stat_kcache", "extension=set_user", "extension=pg_mon"} {
			if !strings.Contains(joined, "--exclude-"+providerObject) {
				t.Fatalf("shared dump restores provider-owned %s: %q", providerObject, joined)
			}
		}
		if strings.Contains(joined, "--exclude-extension=pgcrypto") || strings.Contains(joined, "--exclude-extension=*") {
			t.Fatal("shared dump must preserve application-owned extensions")
		}
		if len(call.input) == 0 {
			t.Fatalf("shared dump credential was not supplied over stdin")
		}
	}
	joined := strings.Join(runtime.calls[0].args, " ") + "\n" + strings.Join(runtime.calls[1].args, " ")
	if !strings.Contains(joined, "-d 'app_a_dev'") || !strings.Contains(joined, "-d 'app_a_dev_analytics'") {
		t.Fatalf("shared backup did not select both app-a databases: %q", joined)
	}
}

func TestRestoreSharedPostgresInstancesUsesOnlyApplicationResources(t *testing.T) {
	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), "shared")
	m := New("app-a", "dev", true, false, false)
	root := t.TempDir()
	shared := SharedBackendFilesAt(root, "baha", m.Environment)
	if err := os.MkdirAll(shared.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	adminRef, err := ensureSharedPostgresCredential(shared.Dir, "provider-admin", "")
	if err != nil {
		t.Fatal(err)
	}
	appRef, err := ensureSharedPostgresCredential(shared.Dir, sharedBackendApplicationKey(m), "default")
	if err != nil {
		t.Fatal(err)
	}
	state := sharedBackendState{
		Version: sharedBackendStateVersion, Environment: m.Environment, PostgresAdminCredential: adminRef, PostgresMembers: 1,
		Applications: map[string]sharedBackendAppState{
			sharedBackendApplicationKey(m): {
				Application: m.Name, Environment: m.Environment,
				SQL: map[string]sharedPostgresResource{"default": {
					Database: "app_a_dev", Username: "baha_app_a_dev", CredentialReference: appRef,
				}},
			},
		},
	}
	if err := writeSharedBackendState(shared.State, state); err != nil {
		t.Fatal(err)
	}
	runtime := &fakePostgresBackupRuntime{verifyResult: map[string]string{
		sharedPostgresService(m.Environment): "baha_app_a_dev\n",
	}}
	backup := []PostgresBackup{{Instance: "default", SQL: []byte("CREATE TABLE sentinel(value text);\n")}}
	if err := RestorePostgresInstancesAt(context.Background(), runtime, m, RuntimeFiles{}, root, "baha", backup); err != nil {
		t.Fatalf("RestorePostgresInstancesAt() error = %v", err)
	}
	if len(runtime.calls) != 3 {
		t.Fatalf("calls = %d, want restore + role/database ownership verification", len(runtime.calls))
	}
	restore := strings.Join(runtime.calls[0].args, " ")
	if !strings.Contains(restore, "-U 'baha_app_a_dev' -d 'app_a_dev'") {
		t.Fatalf("restore escaped application resource: %q", restore)
	}
	if strings.Contains(restore, "baseharbor_admin") {
		t.Fatalf("restore used provider administrator credentials: %q", restore)
	}
}

type postgresBackupShellRuntime struct {
	t     *testing.T
	files RuntimeFiles
	calls int
}

func (r *postgresBackupShellRuntime) ExecProject(ctx context.Context, project, compose, env, service string, args ...string) (string, error) {
	return r.execute(ctx, project, compose, env, nil, service, args...)
}
func (r *postgresBackupShellRuntime) ExecProjectInput(ctx context.Context, project, compose, env string, input []byte, service string, args ...string) (string, error) {
	return r.execute(ctx, project, compose, env, input, service, args...)
}
func (r *postgresBackupShellRuntime) execute(ctx context.Context, project, compose, env string, input []byte, service string, args ...string) (string, error) {
	r.t.Helper()
	if project != r.files.Project || compose != r.files.Compose || env != r.files.Env || service != "postgres-analytics" {
		r.t.Fatal("backup escaped selected Target runtime files or instance")
	}
	if strings.Contains(strings.Join(args, " "), os.Getenv("POSTGRES_PASSWORD")) || strings.Contains(strings.Join(args, " "), "-U baseharbor") {
		r.t.Fatal("credential material or guessed role in transport argv")
	}
	r.calls++
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdin = bytes.NewReader(input)
	output, err := cmd.CombinedOutput()
	if bytes.Contains(output, []byte(os.Getenv("POSTGRES_PASSWORD"))) {
		r.t.Fatal("client output leaked protected SQL credentials")
	}
	return string(output), err
}

func TestPostgresBackupRestoreExecutesRetainedTLSConsumerIdentityAndSelectedOwner(t *testing.T) {
	for _, scenario := range []struct{ name, user, database, password string }{
		{"simple", "retained_consumer", "retained_database", "retained-private-password"},
		{"special-values", `retained 'role" \ $(touch "$BACKUP_SHELL_INJECTION_MARKER")`, `retained db'" host=other-target.invalid sslmode=disable`, "retained p'ass\"word\\\n$(touch \"$BACKUP_SHELL_INJECTION_MARKER\")"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			m := WithSQLInstances(New("same-app", "dev", true, false, false), "analytics")
			files := RuntimeFilesFor(Store{Root: filepath.Join(root, "canonical-deployment-uuid", "state"), Namespace: "selected-target"}, m)
			bin := filepath.Join(root, "bin")
			if err := os.MkdirAll(bin, 0700); err != nil {
				t.Fatal(err)
			}
			restored := filepath.Join(root, "restored.sql")
			t.Setenv("BACKUP_RESTORE_TEST_SQL", restored)
			script := `#!/bin/sh
set -eu
test "$PGPASSWORD" = "$EXPECT_POSTGRES_PASSWORD"
test "$PGUSER" = "$EXPECT_POSTGRES_USER"
test "$PGDATABASE" = "$EXPECT_POSTGRES_DB"
test "$PGHOST" = postgres-analytics
test "$PGPORT" = 5432
test "$PGSSLMODE" = verify-ca
test "$PGSSLROOTCERT" = /run/baseharbor/tls/ca.pem
case "$0" in
  */pg_dump) printf '%s\n' 'CREATE TABLE retained_probe(id integer);' ;;
  */psql)
    case "$*" in
      *"SELECT 1"*) printf '1\n' ;;
      *) cat > "$BACKUP_RESTORE_TEST_SQL" ;;
    esac ;;
  *) exit 91 ;;
esac
`
			for _, name := range []string{"pg_dump", "psql"} {
				if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			for key, value := range map[string]string{"USER": scenario.user, "DB": scenario.database, "PASSWORD": scenario.password} {
				t.Setenv("POSTGRES_"+key, value)
				t.Setenv("EXPECT_POSTGRES_"+key, value)
			}
			t.Setenv("PGSSLMODE", "disable")
			t.Setenv("PGHOST", "other-target.invalid")
			marker := filepath.Join(root, "shell-injection")
			t.Setenv("BACKUP_SHELL_INJECTION_MARKER", marker)
			r := &postgresBackupShellRuntime{t: t, files: files}
			backups, err := DumpPostgresInstances(t.Context(), r, m, files)
			if err != nil {
				t.Fatal(err)
			}
			if err := RestorePostgresInstances(t.Context(), r, m, files, backups); err != nil {
				t.Fatal(err)
			}
			actual, err := os.ReadFile(restored)
			if err != nil || len(backups) != 1 || !bytes.Equal(actual, backups[0].SQL) || r.calls != 3 {
				t.Fatalf("protected dump/restore/verify did not roundtrip selected SQL: calls=%d err=%v", r.calls, err)
			}
			// SELECT failure after a successful import must also fail the restore.
			verifyFailure := strings.Replace(script, `*"SELECT 1"*) printf '1\n' ;;`, `*"SELECT 1"*) exit 94 ;;`, 1)
			if verifyFailure == script {
				t.Fatal("verification negative did not modify its shell scenario")
			}
			if err := os.WriteFile(filepath.Join(bin, "psql"), []byte(verifyFailure), 0700); err != nil {
				t.Fatal(err)
			}
			if err := RestorePostgresInstances(t.Context(), r, m, files, backups); err == nil || !strings.Contains(err.Error(), "verify restored postgres") {
				t.Fatal("failed SQL verification accepted", err)
			}
			// A native command failure must propagate for dump, SQL import and SELECT.
			for _, client := range []string{"pg_dump", "psql"} {
				if err := os.WriteFile(filepath.Join(bin, client), []byte("#!/bin/sh\nexit 93\n"), 0700); err != nil {
					t.Fatal(err)
				}
				if client == "pg_dump" {
					if _, err := DumpPostgresInstances(t.Context(), r, m, files); err == nil {
						t.Fatal("failed native dump accepted")
					}
				} else if err := RestorePostgresInstances(t.Context(), r, m, files, backups); err == nil {
					t.Fatal("failed native SQL restore accepted")
				}
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("protected connection values were evaluated as shell syntax")
			}
		})
	}

}
