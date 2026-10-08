package keycloak

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/providerupgrade"
)

func TestNativeOpsFailClosedWithoutRuntimeHooks(t *testing.T) {
	n := &NativeOps{}
	ctx := context.Background()
	if _, err := n.Inspect(ctx); err == nil {
		t.Fatal("inventory must reject missing Core hook")
	}
	if err := n.CheckUpgradePath(ctx, "26.7.5", "26.8.0"); err == nil {
		t.Fatal("unknown upgrade path accepted")
	}
	if _, err := n.CreateBackup(ctx, "26.7.5"); err == nil {
		t.Fatal("backup without provider hook accepted")
	}
	if err := n.VerifyBackup(ctx, providerupgrade.BackupRef{}); err == nil {
		t.Fatal("missing backup verifier accepted")
	}
	if err := n.ApplySingle(ctx, "26.8.0", "keycloak", "digest"); err == nil {
		t.Fatal("single mutation without hook accepted")
	}
	if err := n.StopAllMembers(ctx); err == nil {
		t.Fatal("HA stop without hook accepted")
	}
	if err := n.ApplyAllMembers(ctx, "26.8.0", "keycloak", "digest"); err == nil {
		t.Fatal("HA mutation without hook accepted")
	}
	if err := n.ReplaceMember(ctx, "keycloak-1", "26.8.0", "keycloak", "digest"); err == nil {
		t.Fatal("rolling mutation without hook accepted")
	}
	if err := n.WaitMemberReady(ctx, "keycloak-1"); err == nil {
		t.Fatal("member readiness without probe accepted")
	}
	if err := n.WaitAllReady(ctx); err == nil {
		t.Fatal("readiness without probe accepted")
	}
	if err := n.VerifyDatabase(ctx); err == nil {
		t.Fatal("SQL verification missing")
	}
	if err := n.VerifyRealmState(ctx); err == nil {
		t.Fatal("realm verification missing")
	}
	if err := n.VerifyOIDCDiscovery(ctx); err == nil {
		t.Fatal("OIDC issuer missing")
	}
	if err := n.VerifyTokenFlow(ctx); err == nil {
		t.Fatal("token verification missing")
	}
	if err := n.RestoreBackup(ctx, providerupgrade.BackupRef{}, "26.7.5"); err == nil {
		t.Fatal("restore without hook accepted")
	}
}

func TestNativeOpsUsesCoreOwnedHook(t *testing.T) {
	called := false
	n := &NativeOps{Hooks: CoreHooks{
		Compatibility: func(_ context.Context, from, to string) error {
			called = true
			if from != "26.7.5" || to != "26.8.0" {
				t.Fatal("unexpected versions")
			}
			return nil
		},
	}}
	if err := n.CheckUpgradePath(context.Background(), "26.7.5", "26.8.0"); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("Core hook never invoked")
	}
}
