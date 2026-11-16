package main

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestDevelopmentIdentityUsesOnlySelectedRealization(t *testing.T) {
	m := application.WithIdentity(application.WithWorkloadComponents(application.New("demo", "dev", false, false, false), "api"))
	for _, tc := range []struct {
		provider          string
		scope             capability.ProviderScope
		gateway, operator bool
	}{
		{"keycloak", capability.ScopeShared, true, true},
		{"keycloak", capability.ScopeApplication, true, false},
		{"external-oidc", capability.ScopeShared, false, false},
	} {
		t.Run(tc.provider+"/"+string(tc.scope), func(t *testing.T) {
			t.Setenv("BASEHARBOR_IDENTITY_PROVIDER", tc.provider)
			t.Setenv(application.ProviderScopeEnv(capability.ProviderKeycloak), string(tc.scope))
			if requiresDevelopmentGateway(m) != tc.gateway || requiresManagedDevelopmentOperatorRealm(m) != tc.operator {
				t.Fatal("deployment selection was replaced with implicit shared managed Identity")
			}
			if tc.provider == "external-oidc" {
				execution := applicationApplyExecution{manifest: m}
				plan := developmentRoutePlan{target: "isolated", appOwner: "app/demo/dev"}
				if err := execution.addDevelopmentIdentityRoutes(context.Background(), &plan); err != nil || len(plan.appRoutes) != 0 || len(plan.groups) != 0 {
					t.Fatalf("external Identity requested a local managed runtime or route: %#v %v", plan, err)
				}
			}
		})
	}
}
