package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

type localAuthorityRuntime struct {
	bhruntime.RuntimeProvider
	kind         bhruntime.ProviderKind
	engine       *bhruntime.DockerEngineObservation
	localErr     error
	observations int
}

func (r *localAuthorityRuntime) Kind() bhruntime.ProviderKind                     { return r.kind }
func (r *localAuthorityRuntime) DockerEngine() *bhruntime.DockerEngineObservation { return r.engine }
func (r *localAuthorityRuntime) VerifyLocalRuntimeAuthority(context.Context) error {
	r.observations++
	return r.localErr
}

func TestLocalWorkloadAuthorityPreservesAllNonlocalAndPolicyBoundaries(t *testing.T) {
	for _, scenario := range []string{"local-cli", "local-stdio", "local-podman", "http", "unmarked", "tenant", "management", "management-operation", "stored-policy", "partial-env-policy", "unknown-podman", "remote-docker", "unverified-engine", "partial-management", "damaged-management", "redirected-owner", "binding-mismatch", "missing-engine"} {
		t.Run(scenario, func(t *testing.T) {
			target := configureTestTarget(t)
			for _, name := range []string{operatorauth.EnvIssuer, operatorauth.EnvClientID, operatorauth.EnvScopes, operatorauth.EnvCallbackPort} {
				t.Setenv(name, "")
			}
			ctx := localWorkloadScope(withLocalInvocation(operatorauth.WithEnforcement(t.Context()), "cli"), "apply")
			native := &localAuthorityRuntime{kind: bhruntime.ProviderDocker, engine: &bhruntime.DockerEngineObservation{Endpoint: "unix:///run/user/1000/docker.sock", Mode: "rootless", DaemonID: "owned-engine", Verified: true}}
			root, err := targetRuntimeStateRoot(target)
			if err != nil {
				t.Fatal(err)
			}
			allowed := scenario == "local-cli" || scenario == "local-stdio" || scenario == "local-podman"
			switch scenario {
			case "local-stdio":
				ctx = withLocalInvocation(ctx, "stdio")
			case "local-podman", "unknown-podman":
				native.kind, native.engine = bhruntime.ProviderPodman, nil
				if scenario == "unknown-podman" {
					native.localErr = errors.New("remote native selection")
				}
			case "http":
				ctx = withoutLocalInvocation(ctx)
			case "unmarked":
				ctx = localWorkloadScope(operatorauth.WithEnforcement(t.Context()), "apply")
			case "tenant":
				ctx = tenancy.WithContext(ctx, &tenancy.Context{TenantID: "tenant"})
			case "management":
				ctx = withCoreAuthority(ctx, target)
			case "management-operation":
				ctx = localWorkloadScope(ctx, "control-plane.up")
			case "stored-policy":
				if err := deployment.SetOperatorAuthForTargetEnvironment(target.Name, "prod", deployment.OperatorAuthEnvironmentConfig{Provider: "external-oidc", Issuer: "https://issuer.example", ClientID: "operator"}); err != nil {
					t.Fatal(err)
				}
			case "partial-env-policy":
				t.Setenv(operatorauth.EnvIssuer, "https://issuer.example")
			case "remote-docker":
				native.engine.Endpoint = "tcp://remote:2376"
			case "unverified-engine":
				native.engine.Verified = false
			case "damaged-management":
				if err := os.MkdirAll(root, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "installation.json"), []byte("damaged"), 0600); err != nil {
					t.Fatal(err)
				}
			case "partial-management":
				steps := coreinstallation.Steps{
					Preflight: func(context.Context, coreinstallation.State) error { return nil },
					SQL: func(context.Context, coreinstallation.State) error {
						return errors.New("interrupted management materialization")
					},
					Secrets: func(context.Context, coreinstallation.State) error {
						t.Fatal("partial fixture proceeded beyond interruption")
						return nil
					},
					Identity: func(context.Context, coreinstallation.State) (string, error) {
						t.Fatal("partial fixture proceeded beyond interruption")
						return "", nil
					},
					Verify: func(context.Context, coreinstallation.State) error {
						t.Fatal("partial fixture proceeded beyond interruption")
						return nil
					},
				}
				_, err := coreinstallation.Run(t.Context(), root, coreinstallation.Spec{Target: target.Name, Runtime: target.RuntimeProvider, MachineRole: coreinstallation.Deployment}, steps)
				if err == nil {
					t.Fatal("partial fixture unexpectedly completed")
				}
				if _, err := coreinstallation.Load(root); err != nil {
					t.Fatal("partial fixture has no valid management owner", err)
				}
			case "redirected-owner":
				if err := os.MkdirAll(filepath.Dir(root), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), root); err != nil {
					t.Fatal(err)
				}
			case "binding-mismatch":
				if err := os.MkdirAll(root, 0700); err != nil {
					t.Fatal(err)
				}
				other := *native.engine
				other.DaemonID = "foreign-engine"
				data, _ := json.Marshal(dockerEngineBinding{Schema: "baseharbor.docker-engine/v1", Target: target.Name, Observation: other})
				if err := os.WriteFile(filepath.Join(root, "docker-engine.json"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			resolve := func(context.Context, deployment.ResolvedTarget) (bhruntime.RuntimeProvider, error) {
				calls++
				if scenario == "missing-engine" {
					return nil, errors.New("engine unavailable")
				}
				return native, nil
			}
			authority, local, err := verifiedLocalWorkloadAuthorityWithRuntime(ctx, target.Name, "prod", resolve)
			if local != allowed || (allowed && err != nil) {
				t.Fatalf("local authority=%t allowed=%t error=%v", local, allowed, err)
			}
			if allowed {
				if authority.Operation != "apply" || authority.Target != target.Name || authority.Environment != "prod" || calls != 1 {
					t.Fatal("proof is not bound to the observed operation/Target/environment")
				}
				if _, err := os.Lstat(filepath.Join(root, "docker-engine.json")); !os.IsNotExist(err) {
					t.Fatal("authorization observation created an engine binding")
				}
			}
			if scenario == "local-podman" && native.observations != 1 {
				t.Fatal("Podman authority inferred from provider kind without actual local observation")
			}
			if _, principal := operatorauth.PrincipalFromContext(ctx); principal {
				t.Fatal("adapter manufactured a principal")
			}
		})
	}
}
