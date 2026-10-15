package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func externalRemovalTransportFixture(t *testing.T) application.ProviderImpactScope {
	t.Helper()
	configureTestTarget(t)
	t.Chdir(t.TempDir())
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	reg := externalprovider.Registration{ID: "team-db", ProviderID: "team/postgresql", Provider: capability.Provider{Kind: capability.ProviderPostgreSQL, Capabilities: []capability.Kind{capability.SQL}}, Endpoint: "postgres://db.example:5432/app", Trust: externalprovider.Trust{Mode: externalprovider.TrustSystem}}
	if err := application.RegisterExternalProviderAt(root, reg); err != nil {
		t.Fatal(err)
	}
	r, err := (capability.RegistryStore{Path: filepath.Join(root, "provider-registry.json")}).Load()
	if err != nil {
		t.Fatal(err)
	}
	store := capability.RegistryStore{Path: filepath.Join(root, "targets", "docker-dev", "provider-registry.json")}
	if err := store.Update(func(target *capability.Registry) error {
		if err := target.Register(r.Instances[0]); err != nil {
			return err
		}
		return target.BindDeployment(capability.Resource{Application: "web", Kind: capability.SQL, Name: "default", Provider: capability.ProviderPostgreSQL}, application.MustNewApplicationID(), "dev", r.Instances[0].ID)
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := application.PlanExternalProviderRemovalAt(root, reg.ID)
	if err != nil {
		t.Fatal(err)
	}
	return plan.Scope
}

func TestExternalProviderRemovalTransportsShareScopeAndExplicitStages(t *testing.T) {
	for _, transport := range []string{"cli", "http", "mcp"} {
		t.Run(transport, func(t *testing.T) {
			scope := externalRemovalTransportFixture(t)
			consent := application.ProviderChangeConsent{PlanID: scope.PlanID, First: true, Second: true}
			encoded, _ := json.Marshal(consent)
			t.Setenv(providerChangeConsentEnv, string(encoded))
			ctx := context.Background()
			var client *mcp.ClientSession
			if transport == "mcp" {
				client = workspaceMutationClient(t)
			}
			invoke := func(supplied *application.ProviderChangeConsent) (json.RawMessage, error) {
				if transport == "cli" {
					var out bytes.Buffer
					err := providerRemoveCommand().Run(withProviderChangeConsent(ctx, supplied), []string{"team-db", "--yes", "-o", "json"}, &out, io.Discard)
					return out.Bytes(), err
				}
				input := machineProviderRemoveInput{ID: "team-db", Approval: true, ProviderChangeConsent: supplied}
				raw, _ := json.Marshal(input)
				if transport == "http" {
					value, err := executeHTTPProviderMutation(ctx, "provider.remove", raw, nil)
					result, _ := json.Marshal(value)
					return result, err
				}
				var args map[string]any
				if err := json.Unmarshal(raw, &args); err != nil {
					return nil, err
				}
				result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.provider.remove", Arguments: args})
				if err != nil {
					return nil, err
				}
				encoded, _ := json.Marshal(result)
				if result.IsError {
					return encoded, errors.New(string(encoded))
				}
				encoded, _ = json.Marshal(result.StructuredContent)
				return encoded, nil
			}
			denied, err := invoke(nil)
			if err == nil {
				t.Fatal("ambient consent/ordinary --yes bypassed explicit stages")
			}
			if transport != "mcp" {
				var domain *machine.Error
				if !errors.As(err, &domain) || domain.Code != machine.ErrorApprovalRequired {
					t.Fatalf("wrong denial: %v", err)
				}
				actual, ok := domain.Details.(application.ProviderImpactScope)
				if !ok || actual.PlanID != scope.PlanID {
					t.Fatal("transport omitted aggregate plan")
				}
			} else if !bytes.Contains(denied, []byte(scope.PlanID)) {
				t.Fatal("MCP omitted same aggregate plan")
			}
			result, err := invoke(&consent)
			if err != nil {
				t.Fatal(err)
			}
			var removed application.ExternalProviderRemovalResult
			if err := json.Unmarshal(result, &removed); err != nil {
				t.Fatal(err)
			}
			if !removed.Removed || removed.ForeignMutated || removed.Scope.Status != "partial" || removed.Scope.PlanID != scope.PlanID {
				t.Fatalf("wrong common final result: %s", result)
			}
		})
	}
}

func TestExternalProviderRemovalChecksEveryManagedConsumerBeforeMutation(t *testing.T) {
	scope := externalRemovalTransportFixture(t)
	root := filepath.Dir(scope.Owner.Reference)
	store := capability.RegistryStore{Path: filepath.Join(root, "targets", "docker-dev", "provider-registry.json")}
	if err := store.Update(func(registry *capability.Registry) error {
		binding := registry.Bindings[0]
		return registry.BindDeployment(binding.Resource, application.MustNewApplicationID(), "prod", binding.ProviderInstanceID)
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := application.PlanExternalProviderRemovalAt(root, "team-db")
	if err != nil {
		t.Fatal(err)
	}
	consent := application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true}
	_, err = removeExternalProviderWithImpact(withProviderChangeConsent(context.Background(), &consent), "team-db", nil, nil)
	var failure *machine.Error
	if !errors.As(err, &failure) || failure.Code != machine.ErrorAuthenticationFailed {
		t.Fatalf("other managed consumer was not authorized: %v", err)
	}
	actual, err := store.Load()
	if err != nil || len(actual.Bindings) != 2 {
		t.Fatal("unauthorized global removal released bindings")
	}
	if _, err := application.InspectExternalProvider("team-db"); err != nil {
		t.Fatal("unauthorized global removal deleted registration")
	}
}

func TestExternalProviderRemovalCannotInferGlobalOwnershipFromTenantRole(t *testing.T) {
	scope := externalRemovalTransportFixture(t)
	consent := application.ProviderChangeConsent{PlanID: scope.PlanID, First: true, Second: true}
	ctx := tenancy.WithContext(context.Background(), &tenancy.Context{TenantID: application.MustNewApplicationID(), ExternalIdentityID: "operator", Roles: []string{"admin"}})
	_, err := removeExternalProviderWithImpact(withProviderChangeConsent(ctx, &consent), "team-db", nil, nil)
	var failure *machine.Error
	if !errors.As(err, &failure) || failure.Code != machine.ErrorOwnershipAmbiguous {
		t.Fatalf("tenant role manufactured global ownership: %v", err)
	}
	if _, err := application.InspectExternalProvider("team-db"); err != nil {
		t.Fatal("unverified tenant owner changed global registration")
	}
}
