package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type postgresReadinessRuntime struct {
	bhruntime.RuntimeProvider
	attempts int
	service  string
	input    string
	args     []string
}

func (r *postgresReadinessRuntime) ExecProjectInput(_ context.Context, _, _, _ string, input []byte, service string, args ...string) (string, error) {
	r.attempts++
	r.service = service
	r.input = string(input)
	r.args = append([]string(nil), args...)
	if r.attempts == 1 {
		return "", errors.New("tls error: EOF")
	}
	return "1", nil
}

func TestWaitForOpenBaoPostgresStorageRetriesTransientTLSEOF(t *testing.T) {
	const password = "openbao-storage-test-secret"
	dir := t.TempDir()
	env := filepath.Join(dir, "runtime.env")
	content := strings.Join([]string{
		"BASEHARBOR_POSTGRES_USER=core",
		"BASEHARBOR_POSTGRES_PASSWORD=core-secret",
		"BASEHARBOR_POSTGRES_INTERNAL_USER=internal",
		"BASEHARBOR_POSTGRES_INTERNAL_PASSWORD=internal-secret",
		"BASEHARBOR_POSTGRES_REPLICATION_USER=replication",
		"BASEHARBOR_POSTGRES_REPLICATION_PASSWORD=replication-secret",
		"BASEHARBOR_OPENBAO_DB_USER=openbao_runtime",
		"BASEHARBOR_OPENBAO_DB_PASSWORD=" + password,
	}, "\n") + "\n"
	if err := os.WriteFile(env, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	runtime := &postgresReadinessRuntime{}
	files := bhruntime.Files{Project: "restart-test", Compose: filepath.Join(dir, "compose.yaml"), Env: env}
	if err := waitForOpenBaoPostgresStorage(context.Background(), runtime, files); err != nil {
		t.Fatal(err)
	}
	if runtime.attempts != 2 {
		t.Fatalf("PostgreSQL readiness probes = %d, want retry after one transient failure", runtime.attempts)
	}
	if runtime.service != "postgres-admin" || runtime.input != password+"\n" {
		t.Fatalf("PostgreSQL probe service/input = %q/%q", runtime.service, runtime.input)
	}
	if !reflect.DeepEqual(runtime.args[len(runtime.args)-2:], []string{"openbao_runtime", "openbao"}) {
		t.Fatalf("PostgreSQL probe identity/database = %v", runtime.args)
	}
	if strings.Contains(strings.Join(runtime.args, " "), password) {
		t.Fatal("OpenBao PostgreSQL password leaked into process arguments")
	}
}
