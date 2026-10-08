package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type openBaoSQLProbeRuntime struct {
	bhruntime.RuntimeProvider
	input   string
	service string
	args    []string
	fail    bool
}

func (r *openBaoSQLProbeRuntime) ExecProjectInput(_ context.Context, _, _, _ string, input []byte, service string, args ...string) (string, error) {
	r.input = string(input)
	r.service = service
	r.args = append([]string(nil), args...)
	if r.fail {
		return "", errors.New("SQL authentication rejected")
	}
	return "1", nil
}

func TestNativeOpenBaoSQLVerificationProtectsCredentials(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "runtime.env")
	const password = "strong-sql-secret-not-in-argv"
	content := strings.Join([]string{
		"BASEHARBOR_POSTGRES_USER=core",
		"BASEHARBOR_POSTGRES_PASSWORD=pw",
		"BASEHARBOR_POSTGRES_INTERNAL_USER=postgres",
		"BASEHARBOR_POSTGRES_INTERNAL_PASSWORD=internal",
		"BASEHARBOR_POSTGRES_REPLICATION_USER=replication",
		"BASEHARBOR_POSTGRES_REPLICATION_PASSWORD=replication-pw",
		"BASEHARBOR_OPENBAO_DB_USER=openbao",
		"BASEHARBOR_OPENBAO_DB_PASSWORD=" + password,
	}, "\n") + "\n"
	if err := os.WriteFile(env, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	runtime := &openBaoSQLProbeRuntime{}
	ops := coreNativeRuntimeOps{runtime: runtime, core: bhruntime.Files{Project: "core", Compose: filepath.Join(dir, "compose.yaml"), Env: env}}
	if err := ops.verifyOpenBaoBackingSQL(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runtime.service != "postgres-admin" || !strings.Contains(strings.Join(runtime.args, " "), "openbao") || !strings.Contains(runtime.input, password) {
		t.Fatalf("OpenBao protected SQL probe missing: service=%s args=%v", runtime.service, runtime.args)
	}
	if strings.Contains(strings.Join(runtime.args, " "), password) {
		t.Fatal("SQL password leaked in process arguments")
	}
	runtime.fail = true
	if err := ops.verifyOpenBaoBackingSQL(context.Background()); err == nil {
		t.Fatal("failed OpenBao SQL authentication accepted")
	}
}
