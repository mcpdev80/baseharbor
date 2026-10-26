package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Reuse the original once-seeded Core fixture, its real TLS SQL marker and
// running sibling. An active binding demonstrates deliberate provider outage;
// it does not claim to be a running application/login/secret consumer journey.
func nativeCoreDestroyRetention(t *testing.T, ctx context.Context, provider bhruntime.RuntimeProvider, files bhruntime.Files, recovery string, config bhruntime.Config, credentials bhruntime.ControlPlaneCredentials, siblingID string, env []byte) {
	t.Helper()
	target, err := effectiveTarget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	root, err := targetDataRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	m := application.New("active-core-destroy-binding", "dev", true, false, false)
	m.ApplicationID = application.MustNewApplicationID()
	if err := application.ReconcileReferenceProviderRegistryAt(root, m); err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(root, "provider-registry.json")
	registry, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runtimeDestroyCommand(withProviderChangeConsent(ctx, nil), []string{"--yes", "--json"}, &out, io.Discard); machine.Classify(err).Code != machine.ErrorApprovalRequired {
		t.Fatal("Core destroy generic approval bypass", err)
	}
	p, err := buildSelectedCoreDestroyPlan(ctx)
	if err != nil {
		t.Fatal("active-binding Core destruction veto", err)
	}
	ack := &application.ProviderChangeConsent{PlanID: p.Impact.Scope.PlanID, First: true, Second: true}
	session := workspaceMutationClient(t)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.control-plane.destroy", Arguments: machineControlPlaneDestroyInput{Approval: true, ProviderChangeConsent: ack}})
	if err != nil || result.IsError {
		t.Fatalf("acknowledged active-binding Core destroy: %+v %v", result, err)
	}
	raw, _ := json.Marshal(result.StructuredContent)
	var report destroyReport
	if json.Unmarshal(raw, &report) != nil || !report.Destroyed || report.ProviderChangeResult == nil || report.ProviderChangeResult.Status != "partial" || !report.ProviderChangeResult.Owner.Verified {
		t.Fatal("false complete consumer consequence", string(raw))
	}
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	siblingRetained := false
	for _, container := range containers {
		if container.ID == siblingID && container.Running {
			siblingRetained = true
		}
		for _, service := range p.Owner.Services {
			if container.Project == files.Project && container.Service == service {
				t.Fatal("Core container or startup unit still materialized", service)
			}
		}
	}
	if !siblingRetained {
		t.Fatal("Core destroy changed project sibling")
	}
	for path, before := range map[string][]byte{files.Env: env, registryPath: registry} {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("Core destroy changed original protected owner", err)
		}
	}
	current, err := buildSelectedCoreDestroyPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	currentAck := &application.ProviderChangeConsent{PlanID: current.Impact.Scope.PlanID, First: true, Second: true}
	if err := runtimeDestroy(withProviderChangeConsent(ctx, currentAck), []string{"--yes"}, io.Discard); err != nil {
		t.Fatal("native removed-Core retry", err)
	}
	if err := runtimeUpExisting(ctx, &out, recovery); err != nil {
		t.Fatal("resume after deliberate Core destroy", err)
	}
	if err := nativeCoreSQLFullTLS(ctx, provider, files, credentials, config.PostgresDB, "DO $$ BEGIN IF (SELECT count(*) FROM core_stop_original WHERE value='original-before-stop') <> 1 THEN RAISE EXCEPTION 'original row missing'; END IF; END $$;"); err != nil {
		t.Fatal("original SQL data after deliberate Core retirement", err)
	}
	t.Log("active-binding deliberate Core destroy/retry/resume: canonical containers removed, volumes/files/bindings and running sibling retained; original TLS SQL row read without reseeding; consumer consequence remains partial")
}

func nativeCoreDeleteData(t *testing.T, ctx context.Context, provider bhruntime.RuntimeProvider, files bhruntime.Files, siblingID string, env []byte) {
	t.Helper()
	ctx = withSelectedCoreDataDeletion(ctx, true)
	p, err := buildSelectedCoreDestroyPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ack := &application.ProviderChangeConsent{PlanID: p.Impact.Scope.PlanID, First: true, Second: true}
	if err := runtimeDestroy(withProviderChangeConsent(ctx, ack), []string{"--yes", "--delete-data"}, io.Discard); machine.Classify(err).Code != machine.ErrorApprovalRequired {
		t.Fatal("native deletion lacked separate consent", err)
	}
	ack.DeleteData = true
	session := workspaceMutationClient(t)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.control-plane.destroy", Arguments: machineControlPlaneDestroyInput{Approval: true, DeleteData: true, ProviderChangeConsent: ack}})
	if err != nil || result.IsError {
		t.Fatalf("native confirmed Core data deletion: %+v %v", result, err)
	}
	actual, err := provider.InspectProjectResources(ctx, files.Project, p.Owner.Resources)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range actual {
		if resource.Kind == "container" || resource.Kind == "volume" {
			t.Fatal("selected resource survived confirmed data deletion", resource)
		}
	}
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	siblingRetained := false
	for _, container := range containers {
		siblingRetained = siblingRetained || (container.ID == siblingID && container.Running)
	}
	if !siblingRetained {
		t.Fatal("data deletion changed unrelated sibling")
	}
	after, err := os.ReadFile(files.Env)
	if err != nil || !bytes.Equal(after, env) {
		t.Fatal("data deletion silently changed original credentials", err)
	}
	for path, revision := range p.Owner.protected {
		data, err := runtimeMutationOwnerBytes(path, true)
		unchanged := err == nil && runtimeMutationRevision(data) == revision
		zeroBytes(data)
		if !unchanged {
			t.Fatal("data deletion discarded retry owner")
		}
	}
	t.Log("native explicit Core data deletion: missing deletion consent refused; two current-plan acknowledgements plus independent data consent remove canonical Core containers/volumes; network/sibling/credentials/consumer owners remain; lost volume contents are not claimed recoverable")
}
