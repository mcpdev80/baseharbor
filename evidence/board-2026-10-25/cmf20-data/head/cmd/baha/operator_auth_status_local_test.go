package main

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

func TestOperatorAuthObservationUsesActualLocalAuthorityBoundaries(t *testing.T) {
	for _, scenario := range []string{"cli", "stdio", "http", "unmarked", "tenant", "management", "stored-policy", "environment-policy", "remote-engine", "unverified-engine"} {
		t.Run(scenario, func(t *testing.T) {
			target := configureTestTarget(t)
			for _, name := range []string{operatorauth.EnvIssuer, operatorauth.EnvClientID, operatorauth.EnvScopes, operatorauth.EnvCallbackPort} {
				t.Setenv(name, "")
			}
			ctx := withLocalInvocation(operatorauth.WithEnforcement(t.Context()), "cli")
			native := &localAuthorityRuntime{kind: bhruntime.ProviderDocker, engine: &bhruntime.DockerEngineObservation{Endpoint: "unix:///run/user/1000/docker.sock", Mode: "rootless", DaemonID: "owned-engine", Verified: true}}
			switch scenario {
			case "stdio":
				ctx = withLocalInvocation(ctx, "stdio")
			case "http":
				ctx = withoutLocalInvocation(ctx)
			case "unmarked":
				ctx = operatorauth.WithEnforcement(t.Context())
			case "tenant":
				ctx = tenancy.WithContext(ctx, &tenancy.Context{TenantID: "tenant"})
			case "management":
				ctx = withCoreAuthority(ctx, target)
			case "stored-policy":
				if err := deployment.SetOperatorAuthForTargetEnvironment(target.Name, "prod", deployment.OperatorAuthEnvironmentConfig{Provider: "external-oidc", Issuer: "https://issuer.example", ClientID: "operator"}); err != nil {
					t.Fatal(err)
				}
			case "environment-policy":
				t.Setenv(operatorauth.EnvIssuer, "https://issuer.example")
			case "remote-engine":
				native.engine.Endpoint = "tcp://remote:2376"
			case "unverified-engine":
				native.engine.Verified = false
			}
			observe := func(ctx context.Context, target, environment string) (operatorauth.LocalRuntimeAuthority, bool, error) {
				return verifiedLocalWorkloadAuthorityWithRuntime(ctx, target, environment, func(context.Context, deployment.ResolvedTarget) (bhruntime.RuntimeProvider, error) {
					return native, nil
				})
			}
			result := collectOperatorAuthObservationWithLocalAuthority(ctx, target.Name, "prod", observe)
			allowed := scenario == "cli" || scenario == "stdio"
			if (result.Mode == "local-runtime" && result.Status == "READY") != allowed {
				t.Fatalf("unexpected local authority observation: %+v", result)
			}
			if allowed && (result.Session != "not-required" || result.Principal != "" || result.Issuer != "" || len(result.Methods) != 0) {
				t.Fatal("local runtime observation manufactured an OIDC session or principal")
			}
			if !allowed && result.Mode != "oidc" {
				t.Fatal("nonlocal or explicit-policy observation lost its OIDC boundary")
			}
		})
	}
}
