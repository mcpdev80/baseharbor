package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

type disconnectAuthorityFixture struct {
	status           targetenrollment.NodeStatus
	reads, mutations int
	beforeRead       func(int)
	mutationError    error
	afterMutation    func()
}

func (a *disconnectAuthorityFixture) Status(context.Context, targetenrollment.Scope) (targetenrollment.NodeStatus, error) {
	a.reads++
	if a.beforeRead != nil {
		a.beforeRead(a.reads)
	}
	return a.status, nil
}
func (a *disconnectAuthorityFixture) Disconnect(context.Context, targetenrollment.Scope) error {
	a.mutations++
	a.status.Revoked = true
	if a.afterMutation != nil {
		a.afterMutation()
	}
	return a.mutationError
}
func disconnectFixture(t *testing.T) (targetenrollment.Scope, *disconnectAuthorityFixture) {
	t.Helper()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "static")
	configureTestTarget(t)
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	scope := targetenrollment.Scope{TenantID: "11111111-1111-4111-8111-111111111111", TargetID: "edge", NodeID: "edge-node", Runtime: "docker"}
	cfg.Access["edge-node"] = deployment.AccessDefinition{Provider: nodeConnectorProvider, Reference: scope.NodeID}
	cfg.Targets[scope.TargetID] = deployment.TargetDefinition{TenantID: scope.TenantID, Runtime: deployment.RuntimeDefinition{Provider: scope.Runtime}, Access: deployment.TargetAccess{Reference: "edge-node"}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	target, err := cfg.ResolveTarget(scope.TargetID, "")
	if err != nil {
		t.Fatal(err)
	}
	m := application.New("web", "prod", false, false, false)
	m.ApplicationID = application.MustNewApplicationID()
	registerTestDeployment(t, target, m, t.TempDir(), "baseharbor.yaml")
	return scope, &disconnectAuthorityFixture{status: targetenrollment.NodeStatus{Enrolled: true, Serial: "private-serial-never-output", ExpiresAt: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}}
}
func disconnectConsent(t *testing.T, scope targetenrollment.Scope, a *disconnectAuthorityFixture) json.RawMessage {
	t.Helper()
	plan, err := planNodeDisconnect(scope, a.status)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func TestNodeDisconnectRequiresBothCurrentAcknowledgementsWithoutMutation(t *testing.T) {
	scope, a := disconnectFixture(t)
	plan, err := planNodeDisconnect(scope, a.status)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(plan)
	if plan.Scope.Complete || plan.Scope.ConfirmationsRequired != 2 || plan.Scope.Attention != "Achtung" || len(plan.Materials) < 2 || len(plan.Functions) < 2 || strings.Contains(string(raw), a.status.Serial) {
		t.Fatal("missing concrete, secret-safe incomplete impact")
	}
	for range 25 {
		again, err := planNodeDisconnect(scope, a.status)
		if err != nil || again.Scope.PlanID != plan.Scope.PlanID {
			t.Fatal("read-only plan is unstable", err)
		}
	}
	for _, consent := range []application.ProviderChangeConsent{{}, {PlanID: plan.Scope.PlanID, First: true}, {PlanID: "stale", First: true, Second: true}, {First: true, Second: true}} {
		raw, _ := json.Marshal(consent)
		_, err := disconnectNodeWithImpact(context.Background(), a, scope, raw)
		if err == nil || a.mutations != 0 {
			t.Fatal("ordinary/stale/incomplete consent revoked node")
		}
	}
	for _, raw := range []string{`{"plan_id":"sentinel-secret","first":false,"first":true,"second":true}`, `{"plan_id":"sentinel-secret","unexpected":"sentinel-secret"}`, "null", "[]"} {
		_, err := disconnectNodeWithImpact(context.Background(), a, scope, json.RawMessage(raw))
		if err == nil || a.mutations != 0 || strings.Contains(err.Error(), "sentinel-secret") {
			t.Fatal("ambiguous consent accepted or leaked")
		}
	}
}
func TestNodeDisconnectRetainsOwnersAndReportsActualRevocation(t *testing.T) {
	scope, a := disconnectFixture(t)
	path, _ := deployment.ConfigPath()
	before, _ := os.ReadFile(path)
	result, err := disconnectNodeWithImpact(context.Background(), a, scope, disconnectConsent(t, scope, a))
	after, _ := os.ReadFile(path)
	if err != nil || a.mutations != 1 || !result.Revoked || !result.CertificateRevoked || !result.OwnerPreserved || result.Deleted || result.DataDeleted || result.State != "disconnected" || result.Scope.Status != "partial" || string(before) != string(after) {
		t.Fatalf("actual disconnect result: %+v %v", result, err)
	}
	records, err := deployment.ListDeployments(scope.TargetID)
	if err != nil || len(records) != 1 {
		t.Fatal("deployment recovery owner was deleted", err)
	}
	for _, material := range result.RetainedMaterials {
		if material.Disposition != "retained" || !material.Verified || material.Location == nil {
			t.Fatal("retention result unverified")
		}
	}
}
func TestNodeDisconnectRevalidatesRenewalAndOwnerChangesBeforeRevocation(t *testing.T) {
	for _, change := range []string{"certificate", "deployment"} {
		t.Run(change, func(t *testing.T) {
			scope, a := disconnectFixture(t)
			consent := disconnectConsent(t, scope, a)
			a.beforeRead = func(read int) {
				if read != 2 {
					return
				}
				if change == "certificate" {
					a.status.Serial = "renewed-private-serial"
					return
				}
				cfg, _ := deployment.LoadConfig()
				cfg.DefaultTarget = scope.TargetID
				if err := cfg.Save(); err != nil {
					t.Fatal(err)
				}
			}
			_, err := disconnectNodeWithImpact(context.Background(), a, scope, consent)
			var failure *machine.Error
			if !errors.As(err, &failure) || failure.Code != machine.ErrorConflict || a.mutations != 0 {
				t.Fatal("stale approved scope revoked node", err)
			}
		})
	}
}
func TestNodeDisconnectReportsPartialIssuerFailureAndAllowsReviewedRetry(t *testing.T) {
	scope, a := disconnectFixture(t)
	a.mutationError = errors.New("sentinel-private-issuer-body")
	result, err := disconnectNodeWithImpact(context.Background(), a, scope, disconnectConsent(t, scope, a))
	if err == nil || !result.Revoked || result.CertificateRevoked || !result.OwnerPreserved || result.State != "partial" || strings.Contains(err.Error(), "sentinel-private") {
		t.Fatal("partial issuer revocation was hidden or leaked", err)
	}
	a.mutationError = nil
	result, err = disconnectNodeWithImpact(context.Background(), a, scope, disconnectConsent(t, scope, a))
	if err != nil || !result.CertificateRevoked || a.mutations != 2 {
		t.Fatal("reviewed retry did not complete certificate revocation", err)
	}
}
func TestNodeDisconnectRejectsForeignRedirectedAndChangedRecoveryOwners(t *testing.T) {
	for _, change := range []string{"tenant", "symlink", "permissions", "after-revocation"} {
		t.Run(change, func(t *testing.T) {
			scope, a := disconnectFixture(t)
			consent := disconnectConsent(t, scope, a)
			path, _ := deployment.ConfigPath()
			switch change {
			case "tenant":
				scope.TenantID = "22222222-2222-4222-8222-222222222222"
			case "symlink":
				data, _ := os.ReadFile(path)
				foreign := filepath.Join(t.TempDir(), "foreign-config")
				os.WriteFile(foreign, data, 0600)
				os.Remove(path)
				os.Symlink(foreign, path)
			case "permissions":
				os.Chmod(path, 0644)
			case "after-revocation":
				a.afterMutation = func() {
					cfg, _ := deployment.LoadConfig()
					cfg.DefaultTarget = scope.TargetID
					if err := cfg.Save(); err != nil {
						t.Fatal(err)
					}
				}
			}
			result, err := disconnectNodeWithImpact(context.Background(), a, scope, consent)
			if err == nil {
				t.Fatal("unverified recovery owner accepted")
			}
			if change == "after-revocation" {
				if !result.Revoked || result.OwnerPreserved || result.State != "partial" {
					t.Fatal("changed final owner reported preserved")
				}
			} else if a.mutations != 0 {
				t.Fatal("foreign owner mutated")
			}
		})
	}
}
