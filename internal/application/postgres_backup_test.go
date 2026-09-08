package application

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type postgresBackupCall struct {
	service string
	args    []string
	input   []byte
}

type fakePostgresBackupRuntime struct {
	dumps        map[string]string
	execErr      map[string]error
	restoreErr   map[string]error
	verifyResult map[string]string
	calls        []postgresBackupCall
}

func (f *fakePostgresBackupRuntime) ExecProject(_ context.Context, _, _, _, service string, args ...string) (string, error) {
	f.calls = append(f.calls, postgresBackupCall{service: service, args: append([]string(nil), args...)})
	if err := f.execErr[service]; err != nil {
		return "", err
	}
	if len(args) > 0 && args[0] == "pg_dump" {
		return f.dumps[service], nil
	}
	if len(args) > 0 && args[0] == "psql" {
		if result, ok := f.verifyResult[service]; ok {
			return result, nil
		}
		return "1\n", nil
	}
	return "", nil
}

func (f *fakePostgresBackupRuntime) ExecProjectInput(_ context.Context, _, _, _ string, input []byte, service string, args ...string) (string, error) {
	f.calls = append(f.calls, postgresBackupCall{service: service, args: append([]string(nil), args...), input: append([]byte(nil), input...)})
	if err := f.restoreErr[service]; err != nil {
		return "", err
	}
	return "", nil
}

func TestDumpPostgresInstancesUsesStableMultiInstanceIdentity(t *testing.T) {
	m := WithPostgresInstances(New("mailflow", "dev", true, false, false), "analytics", "primary")
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
		if !strings.Contains(joined, "pg_dump --clean --if-exists --no-owner --no-privileges --format=plain") {
			t.Fatalf("dump args = %q", joined)
		}
		if strings.Contains(joined, "PASSWORD") {
			t.Fatalf("dump command must not contain a password: %q", joined)
		}
	}
	if got := strings.Join(runtime.calls[0].args, " "); !strings.Contains(got, "-d mailflow_dev_analytics") {
		t.Fatalf("analytics database not selected: %q", got)
	}
	if got := strings.Join(runtime.calls[1].args, " "); !strings.Contains(got, "-d mailflow_dev_primary") {
		t.Fatalf("primary database not selected: %q", got)
	}
}

func TestRestorePostgresInstancesUsesStdinAndVerifiesEachInstance(t *testing.T) {
	m := WithPostgresInstances(New("mailflow", "dev", true, false, false), "analytics", "primary")
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
		if !strings.Contains(joined, "psql -v ON_ERROR_STOP=1") {
			t.Fatalf("restore args = %q", joined)
		}
		if strings.Contains(joined, string(restore.input)) {
			t.Fatalf("restore SQL leaked into process arguments")
		}
		if got := strings.Join(verify.args, " "); !strings.Contains(got, "-tAc SELECT 1") {
			t.Fatalf("verify args = %q", got)
		}
	}
}

func TestValidatePostgresBackupSetFailsClosed(t *testing.T) {
	m := WithPostgresInstances(New("mailflow", "dev", true, false, false), "analytics", "primary")
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
