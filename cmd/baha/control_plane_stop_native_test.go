package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func approvedNativeCoreStop(t *testing.T, ctx context.Context, out io.Writer) error {
	t.Helper()
	plan, err := buildControlPlaneStopPlan(ctx)
	if err != nil {
		return err
	}
	consent := application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true}
	return runtimeDown(withProviderChangeConsent(ctx, &consent), out)
}

func TestCoreStopRetainsOriginalSQLCredentialsAndRunningSibling(t *testing.T) {
	if os.Getenv("BASEHARBOR_CORE_STOP_ACCEPTANCE") != "1" {
		t.Skip("native Core stop acceptance is opt-in")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Minute)
	defer cancel()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")
	kind := strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME"))
	if kind == "" {
		kind = "docker"
	}
	var out bytes.Buffer
	if err := runWithIO(ctx, []string{"target", "create", "core-stop-native", "--provider", kind, "--access", "core-stop-native", "--reference", "local", "--default"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	pg, err := selectControlPlanePort(&out, "PostgreSQL", "--postgres-port", 0, bhruntime.DefaultPostgresPort, 15432)
	if err != nil {
		t.Fatal(err)
	}
	bao, err := selectControlPlanePort(&out, "OpenBao", "--openbao-port", 0, bhruntime.DefaultOpenBaoPort, 18200)
	if err != nil {
		t.Fatal(err)
	}
	ha := os.Getenv("BASEHARBOR_CORE_STOP_HA_ACCEPTANCE") == "1"
	t.Logf("selected Core lifecycle topology: HA=%t", ha)
	if err := runtimeUpWithPorts(ctx, &out, bhruntime.Ports{Postgres: pg, OpenBao: bao}, ha); err != nil {
		t.Fatalf("Core start: %v\n%s", err, out.String())
	}
	files, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	sibling := []bhruntime.ProjectResource{{Kind: "container", Name: files.Project + "-preserved-sibling-1"}}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := provider.DestroyOwnedProjectResources(cleanup, files.Project, sibling); err != nil {
			t.Errorf("sibling cleanup: %v", err)
		}
		if err := approvedNativeFixtureCleanup(t, cleanup, io.Discard); err != nil {
			t.Errorf("Core cleanup: %v", err)
		}
	}()
	recovery := filepath.Join(t.TempDir(), "openbao-recovery.json")
	if err := platformopenbao.Bootstrap(ctx, provider, files, recovery); err != nil {
		t.Fatal(err)
	}
	if err := reconcileControlPlaneServiceAccess(ctx, provider, files, recovery); err != nil {
		t.Fatal(err)
	}
	config, err := bhruntime.LoadConfig(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := bhruntime.LoadControlPlaneCredentials(files)
	if err != nil {
		t.Fatal(err)
	}
	originalManager, err := platformopenbao.LoadAdminCredentials(files)
	if err != nil {
		t.Fatal(err)
	}
	if err := nativeCoreSQLFullTLS(ctx, provider, files, credentials, config.PostgresDB, "CREATE TABLE core_stop_original (value text); INSERT INTO core_stop_original VALUES ('original-before-stop');"); err != nil {
		t.Fatal("single original SQL seed", err)
	}
	env, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	siblingDir := t.TempDir()
	siblingCompose := filepath.Join(siblingDir, "compose.yaml")
	definition := "services:\n  preserved-sibling:\n    image: docker.io/library/alpine:3.24\n    user: '65000:65000'\n    read_only: true\n    network_mode: none\n    cap_drop: [ALL]\n    security_opt: [no-new-privileges:true]\n    command: ['sh', '-ec', 'exec sleep 900']\n"
	if err := os.WriteFile(siblingCompose, []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	if err := provider.UpProjectFiles(ctx, files.Project, siblingDir, siblingCompose); err != nil {
		t.Fatal("shared-project sibling start", err)
	}
	before, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var siblingBefore bhruntime.RuntimeContainer
	for _, container := range before {
		if container.Project == files.Project {
			t.Logf("before stop: service=%s container=%s state=%s running=%t", container.Service, container.Name, container.State, container.Running)
		}
		if container.Project == files.Project && container.Service == "preserved-sibling" {
			siblingBefore = container
		}
	}
	if siblingBefore.ID == "" || !siblingBefore.Running {
		t.Fatal("sibling not verified running")
	}
	out.Reset()
	err = runWithIO(withProviderChangeConsent(ctx, nil), []string{"down", "--json"}, &out, io.Discard)
	var failure *machine.Error
	if !errors.As(err, &failure) || failure.Code != machine.ErrorApprovalRequired || out.Len() != 0 {
		t.Fatal("native CLI omitted stop acknowledgement bypass", err)
	}
	plan, err := buildControlPlaneStopPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	consent := &application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true}
	session := workspaceMutationClient(t)
	response, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.control-plane.stop", Arguments: machineControlPlaneStopInput{ProviderChangeConsent: consent}})
	if err != nil || response.IsError {
		observed, observeErr := provider.ListRuntimeContainers(ctx)
		if observeErr != nil {
			t.Log("post-failure process inspection failed")
		}
		for _, container := range observed {
			if container.Project == files.Project {
				t.Logf("after failed stop: service=%s container=%s state=%s running=%t", container.Service, container.Name, container.State, container.Running)
			}
		}
		t.Fatalf("native MCP selected stop: %+v %v", response, err)
	}
	raw, _ := json.Marshal(response.StructuredContent)
	var result controlPlaneStopResult
	if json.Unmarshal(raw, &result) != nil || result.State != "stopped" || !result.Runtime.Verified || !result.ProviderChangeResult.Owner.Verified || result.ProviderChangeResult.Status != "partial" {
		t.Fatal("false complete native result", string(raw))
	}
	if err := nativeCoreStopRetry(ctx, session, plan.Scope.PlanID); err != nil {
		t.Fatal("current stopped/removed retry", err)
	}
	after, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	preserved := false
	for _, container := range after {
		if container.ID == siblingBefore.ID && container.Service == siblingBefore.Service && container.Running {
			preserved = true
		}
	}
	if !preserved {
		t.Fatal("selected Core stop changed independently owned sibling")
	}
	currentEnv, err := os.ReadFile(files.Env)
	if err != nil || !bytes.Equal(env, currentEnv) {
		t.Fatal("stop changed original credentials")
	}
	t.Log("native CLI refusal and MCP confirmed Core stop verified; original credential environment, resources and running consolidated-project sibling retained; consumer receipt remains partial")
	if err := runtimeUpExisting(ctx, &out, recovery); err != nil {
		t.Fatalf("Core resume: %v\n%s", err, out.String())
	}
	if err := nativeCoreSQLFullTLS(ctx, provider, files, credentials, config.PostgresDB, "DO $$ BEGIN IF (SELECT count(*) FROM core_stop_original WHERE value='original-before-stop') <> 1 THEN RAISE EXCEPTION 'original row missing'; END IF; END $$;"); err != nil {
		t.Fatal("original SQL row after resume", err)
	}
	if err := platformopenbao.CheckManager(ctx, provider, files); err != nil {
		t.Fatal("original OpenBao manager after resume", err)
	}
	manager, err := platformopenbao.LoadAdminCredentials(files)
	if err != nil || manager.RoleID != originalManager.RoleID || manager.SecretID != originalManager.SecretID {
		t.Fatal("resume changed original OpenBao manager credentials")
	}
	currentEnv, err = os.ReadFile(files.Env)
	if err != nil || !bytes.Equal(env, currentEnv) {
		t.Fatal("resume changed original credentials")
	}
	t.Log("original SQL row and credentials read over verify-full TLS after normal Core resume; unchanged original OpenBao manager usable; no reseeding or rotation")
	nativeCoreDestroyRetention(t, ctx, provider, files, recovery, config, credentials, siblingBefore.ID, env)
	nativeTargetDefinitionRecovery(t, ctx, session, provider, files.Project, target.Name, func() {
		if err := nativeCoreSQLFullTLS(ctx, provider, files, credentials, config.PostgresDB, "DO $$ BEGIN IF (SELECT count(*) FROM core_stop_original WHERE value='original-before-stop') <> 1 THEN RAISE EXCEPTION 'original data changed'; END IF; END $$;"); err != nil {
			t.Fatal("original SQL/TLS data after Target configuration change", err)
		}
		if err := platformopenbao.CheckManager(ctx, provider, files); err != nil {
			t.Fatal("original OpenBao manager after Target configuration change", err)
		}
		manager, err := platformopenbao.LoadAdminCredentials(files)
		if err != nil || manager.RoleID != originalManager.RoleID || manager.SecretID != originalManager.SecretID {
			t.Fatal("Target configuration change rotated original OpenBao credentials")
		}
		actual, err := os.ReadFile(files.Env)
		if err != nil || !bytes.Equal(actual, env) {
			t.Fatal("Target configuration change altered original runtime credentials")
		}
	})
	nativeCoreDeleteData(t, ctx, provider, files, siblingBefore.ID, env)
}

func nativeCoreStopRetry(ctx context.Context, session *mcp.ClientSession, oldID string) error {
	current, err := buildControlPlaneStopPlan(ctx)
	if err != nil {
		return err
	}
	if !current.RuntimeObserved || current.Scope.PlanID == oldID {
		return errors.New("changed native process state retained the old acknowledgement")
	}
	old := &application.ProviderChangeConsent{PlanID: oldID, First: true, Second: true}
	response, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.control-plane.stop", Arguments: machineControlPlaneStopInput{ProviderChangeConsent: old}})
	if err != nil {
		return err
	}
	if !response.IsError {
		return errors.New("old native-state acknowledgement bypassed current plan")
	}
	rawFailure, _ := json.Marshal(response.StructuredContent)
	var failure machineToolError
	if json.Unmarshal(rawFailure, &failure) != nil || failure.Error == nil || failure.Error.CauseCode != "stale-impact-consent" {
		return errors.New("old native-state acknowledgement failed without the required stale-plan cause")
	}
	ack := &application.ProviderChangeConsent{PlanID: current.Scope.PlanID, First: true, Second: true}
	response, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.control-plane.stop", Arguments: machineControlPlaneStopInput{ProviderChangeConsent: ack}})
	if err != nil {
		return err
	}
	if response.IsError {
		return errors.New("explicit current-plan retry of stopped/removed Core failed")
	}
	raw, _ := json.Marshal(response.StructuredContent)
	var result controlPlaneStopResult
	if json.Unmarshal(raw, &result) != nil || result.State != "stopped" || !result.Runtime.Verified || len(result.Runtime.Removed) != 0 {
		return errors.New("stop retry rematerialized or reported unverified resources")
	}
	return nil
}

func nativeCoreSQLFullTLS(ctx context.Context, provider bhruntime.RuntimeProvider, files bhruntime.Files, credentials bhruntime.ControlPlaneCredentials, database, sql string) error {
	const script = "IFS= read -r PGPASSWORD\nexport PGPASSWORD\nexport PGSSLMODE=verify-full PGSSLROOTCERT=/run/baseharbor/postgres-ca/ca.pem\nexec psql -h postgres -p 5432 -U \"$1\" -d \"$2\" -v ON_ERROR_STOP=1"
	input := []byte(credentials.PostgresPassword + "\n" + sql + "\n")
	defer zeroBytes(input)
	_, err := provider.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, input, "postgres-admin", "sh", "-ec", script, "--", credentials.PostgresUser, database)
	return err
}
