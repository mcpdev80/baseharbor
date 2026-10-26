package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type physicalPostgresOwnerRuntime struct {
	bhruntime.RuntimeProvider
	services []string
	role     string
}

func (r *physicalPostgresOwnerRuntime) inspect(service string, args []string) (string, error) {
	r.services = append(r.services, service)
	if service != "shared-postgres-core" {
		return "", errors.New("execution selected the logical app environment instead of the physical provider owner")
	}
	command := strings.Join(args, " ")
	if strings.Contains(command, "SELECT r.rolname") {
		return r.role + "\n", nil
	}
	if strings.Contains(command, "SELECT 1") {
		if strings.Contains(command, "-d postgres") {
			return "", errors.New("application role cannot access provider administration database")
		}
		return "1\n", nil
	}
	if strings.Contains(command, "pg_dump") {
		return "SELECT 1;\n", nil
	}
	return "", nil
}

func (r *physicalPostgresOwnerRuntime) ExecProject(_ context.Context, _, _, _, service string, args ...string) (string, error) {
	return r.inspect(service, args)
}

func (r *physicalPostgresOwnerRuntime) ExecProjectInput(_ context.Context, _, _, _ string, _ []byte, service string, args ...string) (string, error) {
	return r.inspect(service, args)
}

func TestStandaloneSharedPostgresUsesPhysicalOwnerAcrossAppAndRecoveryLifecycle(t *testing.T) {
	root := t.TempDir()
	m := New("owned-app", "dev", true, false, false)
	shared := SharedBackendFilesAt(root, "isolated", m.Environment)
	adminRef, err := ensureSharedPostgresCredential(shared.Dir, "provider-admin", "")
	if err != nil {
		t.Fatal(err)
	}
	credential, err := ensureSharedPostgresCredential(shared.Dir, sharedBackendApplicationKey(m), "default")
	if err != nil {
		t.Fatal(err)
	}
	resource := sharedPostgresResource{Database: sharedPostgresDatabaseName(m, "default"), Username: sharedPostgresRoleName(m, "default"), CredentialReference: credential}
	app := sharedBackendAppState{Application: m.Name, Environment: m.Environment, SQL: map[string]sharedPostgresResource{"default": resource}}
	state := sharedBackendState{Version: sharedBackendStateVersion, Environment: "core", PostgresMembers: 1, PostgresAdminCredential: adminRef, Applications: map[string]sharedBackendAppState{sharedBackendApplicationKey(m): app}}
	if err := writeSharedBackendState(shared.State, state); err != nil {
		t.Fatal(err)
	}
	runtime := &physicalPostgresOwnerRuntime{role: resource.Username}
	ctx := context.Background()
	if err := reconcileSharedPostgresApplication(ctx, runtime, shared, app); err != nil {
		t.Fatalf("resource reconciliation: %v", err)
	}
	if err := VerifySharedPostgreSQL(ctx, runtime, root, "isolated", m); err != nil {
		t.Fatalf("application access/isolation verification: %v", err)
	}
	backups, err := DumpPostgresInstancesAt(ctx, runtime, m, RuntimeFiles{}, root, "isolated")
	if err != nil || len(backups) != 1 {
		t.Fatalf("owned recovery backup: %v", err)
	}
	if err := RestorePostgresInstancesAt(ctx, runtime, m, RuntimeFiles{}, root, "isolated", backups); err != nil {
		t.Fatalf("owned recovery restore/ownership verification: %v", err)
	}
	retained, err := loadSharedBackendState(shared.State, m.Environment)
	if err != nil || retained.Environment != "core" || retained.Applications[sharedBackendApplicationKey(m)].Environment != "dev" {
		t.Fatal("physical execution changed logical application ownership")
	}
	if len(runtime.services) < 8 {
		t.Fatal("lifecycle did not verify actual access, isolation and recovery ownership")
	}
}
