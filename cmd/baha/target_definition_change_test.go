package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

func targetDefinitionFixture(t *testing.T) (string, string) {
	t.Helper()
	target := configureTestTarget(t)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	owner := filepath.Join(root, "retained-runtime-owner")
	if err := os.WriteFile(owner, []byte("original-before-detach"), 0600); err != nil {
		t.Fatal(err)
	}
	m := application.New("consumer", "dev", false, false, false)
	m.ApplicationID = application.MustNewApplicationID()
	registerTestDeployment(t, target, m, t.TempDir(), "baseharbor.yaml")
	return target.Name, owner
}

func TestTargetDefinitionRejectsUnprovenTenantOwnership(t *testing.T) {
	name, _ := targetDefinitionFixture(t)
	ctx := operatorauth.WithVerifiedPrincipal(context.Background(), &identity.Principal{Issuer: "https://issuer.example", Subject: "synthetic-operator"})
	ctx = tenancy.WithContext(ctx, &tenancy.Context{TenantID: "11111111-1111-4111-8111-111111111111", ExternalIdentityID: "synthetic-operator", Roles: []string{"editor"}})
	if _, err := buildTargetDefinitionChangePlan(ctx, name, ""); err == nil || machine.Classify(err).CauseCode != "target_definition_tenant_owner_unverifiable" {
		t.Fatal("tenant acquired unowned global Target definition")
	}
	cfg, err := deployment.LoadConfig()
	if err != nil || cfg.Targets[name].Runtime.Provider != "docker" {
		t.Fatal("ownership refusal changed actual Target", err)
	}
}

func TestTargetDefinitionHonoursYesOnNonTTYForConfigOnlyDetach(t *testing.T) {
	name, owner := targetDefinitionFixture(t)
	plan, err := buildTargetDefinitionChangePlan(context.Background(), name, "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Scope.Destructive {
		t.Fatal("config-only detach must not be destructive")
	}
	ctx := withAssumeYes(context.Background(), true)
	var out bytes.Buffer
	if err := deleteTarget(ctx, []string{name}, &out, &out); err != nil {
		t.Fatalf("--yes on non-TTY must honour both consequence acknowledgements and detach: %v", err)
	}
	if _, err := os.Stat(plan.RecoveryFile); err != nil {
		t.Fatalf("detach must persist the recovery file: %v", err)
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatalf("load config after detach: %v", err)
	}
	if _, exists := cfg.Targets[name]; exists {
		t.Fatal("--yes detach did not remove the configuration entry")
	}
	after, err := os.ReadFile(owner)
	if err != nil || string(after) != "original-before-detach" {
		t.Fatal("--yes detach must not mutate the retained runtime owner")
	}
}

func TestTargetDefinitionRefusesDetachWithoutYesOnNonTTY(t *testing.T) {
	name, owner := targetDefinitionFixture(t)
	ctx := withAssumeYes(context.Background(), false)
	var out bytes.Buffer
	err := deleteTarget(ctx, []string{name}, &out, &out)
	if err == nil || machine.Classify(err).CauseCode != "impact-confirmation-required" {
		t.Fatalf("no --yes on non-TTY must fail closed: %v", err)
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := cfg.Targets[name]; !exists {
		t.Fatal("refusal must not remove the configuration entry")
	}
	if after, err := os.ReadFile(owner); err != nil || string(after) != "original-before-detach" {
		t.Fatal("refusal must not mutate the retained runtime owner")
	}
}

func TestTargetDefinitionRequiresTwoCurrentAcknowledgements(t *testing.T) {
	name, owner := targetDefinitionFixture(t)
	plan, err := buildTargetDefinitionChangePlan(context.Background(), name, "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Scope.Attention != "Achtung" || plan.Scope.ConfirmationsRequired != 2 || plan.Scope.Complete || len(plan.Functions) < 2 {
		t.Fatal("unverified access/consumer consequences not disclosed")
	}
	for _, consent := range []*application.ProviderChangeConsent{nil, {PlanID: plan.Scope.PlanID, First: true}, {PlanID: plan.Scope.PlanID, Second: true}, {PlanID: "stale", First: true, Second: true}} {
		if _, err := deleteTargetDefinition(withProviderChangeConsent(context.Background(), consent), name); err == nil {
			t.Fatal("incomplete/stale acknowledgement accepted")
		}
	}
	if _, err := os.Stat(plan.RecoveryFile); !os.IsNotExist(err) {
		t.Fatal("read-only refusal created recovery", err)
	}
	after, err := os.ReadFile(owner)
	if err != nil || string(after) != "original-before-detach" {
		t.Fatal("refusal mutated existing owner", err)
	}
}

func TestTargetDefinitionStaleMaterialCannotAuthorizeDetach(t *testing.T) {
	name, owner := targetDefinitionFixture(t)
	plan, err := buildTargetDefinitionChangePlan(context.Background(), name, "")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(owner, []byte("modified-before-detach"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(owner, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	consent := application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true}
	_, err = deleteTargetDefinition(withProviderChangeConsent(context.Background(), &consent), name)
	if err == nil || machine.Classify(err).CauseCode != "stale-impact-consent" {
		t.Fatal("changed owner accepted", err)
	}
	cfg, err := deployment.LoadConfig()
	if err != nil || cfg.Targets[name].Runtime.Provider != "docker" {
		t.Fatal("stale detach removed configuration", err)
	}
}

func TestTargetDefinitionDetachAndRestorePreserveAppliedOwners(t *testing.T) {
	name, owner := targetDefinitionFixture(t)
	before, err := deployment.ListDeployments(name)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	original := cfg.Targets[name]
	plan, err := buildTargetDefinitionChangePlan(context.Background(), name, "")
	if err != nil {
		t.Fatal(err)
	}
	ack := application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true}
	var out bytes.Buffer
	if err := deleteTarget(withProviderChangeConsent(context.Background(), &ack), []string{name, "-o", "json"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	var result targetMutationResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || !result.Deleted || result.ProviderChangeResult == nil || result.ProviderChangeResult.Status != "partial" {
		t.Fatal("false complete target result", err)
	}
	if _, err := effectiveTarget(context.Background()); err == nil || machine.Classify(err).CauseCode != "target_selection_detached" {
		t.Fatal("detachment silently switched runtime", err)
	}
	after, err := deployment.ListDeployments(name)
	beforeRaw, _ := json.Marshal(before)
	afterRaw, _ := json.Marshal(after)
	if err != nil || !bytes.Equal(beforeRaw, afterRaw) {
		t.Fatal("applied deployment owners changed", err)
	}
	restore, err := buildTargetDefinitionChangePlan(context.Background(), name, result.RecoveryFile)
	if err != nil {
		t.Fatal(err)
	}
	ack.PlanID = restore.Scope.PlanID
	out.Reset()
	if err := createTarget(withProviderChangeConsent(context.Background(), &ack), []string{name, "--from-recovery", result.RecoveryFile, "-o", "json"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	cfg, err = deployment.LoadConfig()
	a, _ := json.Marshal(cfg.Targets[name])
	b, _ := json.Marshal(original)
	if err != nil || !bytes.Equal(a, b) {
		t.Fatal("restore changed original binding", err)
	}
	if _, err := effectiveTarget(context.Background()); err == nil {
		t.Fatal("restore silently activated an access route")
	}
	if err := writePersistedTarget(name); err != nil {
		t.Fatal(err)
	}
	if selected, err := effectiveTarget(context.Background()); err != nil || selected.Name != name {
		t.Fatal("explicit selection after restore failed", err)
	}
	if after, err := os.ReadFile(owner); err != nil || string(after) != "original-before-detach" {
		t.Fatal("restore changed original runtime owner", err)
	}
}
