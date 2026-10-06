package main

import (
	"context"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

func TestConnectorEnrollmentScopeUsesCoreOwnershipAndTypedAccess(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")
	const owner = "11111111-1111-4111-8111-111111111111"
	cfg := deployment.Config{
		Version: deployment.ConfigVersion,
		Access: map[string]deployment.AccessDefinition{
			"connector": {Provider: "baseharbor-node-connector", Reference: "node-a"},
			"local": {Provider: "local", Reference: "local"},
			"native": {Provider: "native-api", Reference: "api"},
		},
		Targets: map[string]deployment.TargetDefinition{
			"lab": {TenantID: owner, Runtime: deployment.RuntimeDefinition{Provider: "docker"}, Access: deployment.TargetAccess{Reference: "connector"}},
			"unbound": {Runtime: deployment.RuntimeDefinition{Provider: "docker"}, Access: deployment.TargetAccess{Reference: "connector"}},
			"local": {TenantID: owner, Runtime: deployment.RuntimeDefinition{Provider: "docker"}, Access: deployment.TargetAccess{Reference: "local"}},
			"native": {TenantID: owner, Runtime: deployment.RuntimeDefinition{Provider: "docker"}, Access: deployment.TargetAccess{Reference: "native"}},
			"unsupported": {TenantID: owner, Runtime: deployment.RuntimeDefinition{Provider: "kubernetes"}, Access: deployment.TargetAccess{Reference: "connector"}},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	ctx := tenancy.WithContext(context.Background(), &tenancy.Context{TenantID: owner, ExternalIdentityID: "identity-a", Roles: []string{"editor"}})
	scope, err := resolveConnectorEnrollmentScope(ctx, "lab", "node-a", "dev")
	if err != nil || scope != (targetenrollment.Scope{TenantID: owner, TargetID: "lab", NodeID: "node-a", Runtime: "docker"}) {
		t.Fatal("Core-owned remote Target did not resolve exactly", scope, err)
	}
	for _, target := range []string{"missing", "unbound", "local", "native", "unsupported"} {
		if _, err := resolveConnectorEnrollmentScope(ctx, target, "node-a", "dev"); !errors.Is(err, targetenrollment.ErrDenied) {
			t.Fatal("unowned or unsupported Target admitted", target, err)
		}
	}
	foreign := tenancy.WithContext(ctx, &tenancy.Context{TenantID: "22222222-2222-4222-8222-222222222222", ExternalIdentityID: "identity-b", Roles: []string{"editor"}})
	for _, denied := range []context.Context{context.Background(), foreign, tenancy.WithContext(ctx, &tenancy.Context{TenantID: owner})} {
		if _, err := resolveConnectorEnrollmentScope(denied, "lab", "node-a", "dev"); !errors.Is(err, targetenrollment.ErrDenied) {
			t.Fatal("missing or foreign tenant binding admitted", err)
		}
	}
	if _, err := resolveConnectorEnrollmentScope(ctx, "lab", "../escape", "dev"); !errors.Is(err, targetenrollment.ErrDenied) {
		t.Fatal("unbounded node identity admitted", err)
	}
	if _, err := resolveConnectorEnrollmentScope(ctx, "lab", "node-a", "DEV"); !errors.Is(err, targetenrollment.ErrDenied) {
		t.Fatal("noncanonical environment admitted", err)
	}
}

func TestConnectorAuthorityRequiresExplicitTarget(t *testing.T) {
	e := &bahaMachineExecutor{}
	if _, err := e.ConnectorEnrollmentHTTP(context.Background(), nil, ""); err == nil {
		t.Fatal("implicit authority Target admitted")
	}
}
