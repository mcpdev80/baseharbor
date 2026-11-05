package main

import (
	"bytes"
	"context"
	"errors"
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

func TestRepositoryRuntimeInitRecordsPendingDeploymentBeforeStateMutation(t *testing.T) {
	target, repo, manifest, resolved := prepareRepositoryRuntimeInitFirstRun(t)
	principal := &identity.Principal{
		Subject: "authenticated-fixture", Issuer: "https://issuer.example",
		IssuerVerified: true, Groups: []string{"editor"},
	}
	ctx := operatorauth.WithVerifiedPrincipal(context.Background(), principal)
	ctx = operatorauth.WithMembershipProvider(ctx, &firstRunMembershipProvider{
		memberships: []tenancy.Membership{{TenantID: "tenant-a", Role: "editor"}},
	})

	var out bytes.Buffer
	if err := runRepositoryRuntimeInitResolved(ctx, resolved, repo, repositoryInitOptions{Yes: true}, &out); err != nil {
		t.Fatalf("initialize repository deployment inputs with current claim membership: %v\n%s", err, out.String())
	}

	record, found, err := deployment.FindDeployment(target.Name, manifest.ApplicationID, manifest.Environment)
	if err != nil {
		t.Fatalf("find deployment after input initialization: %v", err)
	}
	if !found {
		t.Fatal("deployment record was not created before deployment-local state")
	}
	if record.Observed.State != "configured" || record.Observed.Ready || !record.Observed.VerifiedAt.IsZero() {
		t.Fatalf("configuration falsely claims apply/verification: %#v", record.Observed)
	}
	if record.Identity.Application != manifest.Name || record.Identity.Environment != manifest.Environment {
		t.Fatalf("deployment identity = %#v", record.Identity)
	}
	root, err := deployment.DeploymentRoot(record.Identity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "deployment.json")); err != nil {
		t.Fatalf("deployment record missing: %v", err)
	}
	if _, err := os.Stat(repositoryInitEnvPathFromStateRoot(root)); err != nil {
		t.Fatalf("deployment input state missing: %v", err)
	}

	if _, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "up"); err != nil {
		t.Fatalf("repository resolve after first-run input initialization: %v", err)
	}
}

func TestRepositoryRuntimeInitRequiresCurrentClaimMembershipBeforeStateMutation(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider bool
		revoke   bool
	}{
		{name: "no provider"},
		{name: "provider mapping revoked", provider: true, revoke: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			target, repo, manifest, resolved := prepareRepositoryRuntimeInitFirstRun(t)
			principal := &identity.Principal{
				Subject: "authenticated-fixture", Issuer: "https://issuer.example",
				IssuerVerified: true, Groups: []string{"editor"},
			}
			ctx := operatorauth.WithVerifiedPrincipal(context.Background(), principal)
			provider := &firstRunMembershipProvider{
				memberships: []tenancy.Membership{{TenantID: "tenant-a", Role: "editor"}},
			}
			if test.provider {
				ctx = operatorauth.WithMembershipProvider(ctx, provider)
			}
			if test.revoke {
				operation, ok := machine.OperationByID("app.configure")
				if !ok {
					t.Fatal("app.configure operation is not registered")
				}
				decision, err := operatorauth.AuthorizeMachineOperation(ctx, operatorauth.AuthorizationRequest{
					Operation: operation,
					Context: operatorauth.OperationContext{
						Application: manifest.ApplicationID, Environment: manifest.Environment,
						Target: target.Name, Workspace: repo,
					},
				})
				if err != nil || !decision.Allowed {
					t.Fatalf("live mapping was not authorized before revocation: %#v %v", decision, err)
				}
				provider.memberships = nil
			}

			var out bytes.Buffer
			err := runRepositoryRuntimeInitResolved(ctx, resolved, repo, repositoryInitOptions{Yes: true}, &out)
			if err == nil || machine.Classify(err).Code != machine.ErrorPolicyDenied {
				t.Fatalf("current membership must be required before initialization, got %v\n%s", err, out.String())
			}

			if _, found, err := deployment.FindDeployment(target.Name, manifest.ApplicationID, manifest.Environment); err != nil {
				t.Fatalf("check deployment after denied initialization: %v", err)
			} else if found {
				t.Fatal("denied authorization created a deployment record")
			}
			for _, path := range []string{
				filepath.Join(resolved.DeploymentStateRoot, "deployment.json"),
				repositoryInitEnvPathFromStateRoot(resolved.DeploymentStateRoot),
			} {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("denied authorization changed local state at %s: %v", path, err)
				}
			}
		})
	}
}

func prepareRepositoryRuntimeInitFirstRun(t *testing.T) (deployment.ResolvedTarget, string, application.Manifest, resolvedApplication) {
	t.Helper()
	target := configureTestTargetWithOwner(t)
	repo := t.TempDir()
	manifest := application.Manifest{
		Version:       application.CurrentVersion,
		ApplicationID: application.MustNewApplicationID(),
		Name:          "demo",
		Environment:   "test",
		Workload:      application.WorkloadConfig{Components: []string{"app"}},
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(repo, application.RepositoryManifestName)
	if err := writeInitializedAuthoredManifest(t, manifestPath, manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "compose.yaml"), []byte("services:\n  app:\n    image: alpine:3.20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Errorf("restore test working directory: %v", err)
		}
	})
	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "up")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.DeploymentRecord != nil {
		t.Fatal("fresh repository unexpectedly has a deployment record")
	}
	return target, repo, manifest, resolved
}

type firstRunMembershipProvider struct {
	memberships []tenancy.Membership
}

func (p *firstRunMembershipProvider) ResolveClaimMembership(_ context.Context, principal *identity.Principal) ([]tenancy.Membership, error) {
	if principal == nil || !principal.IssuerVerified {
		return nil, operatorauth.ErrClaimsNotVerified
	}
	return append([]tenancy.Membership(nil), p.memberships...), nil
}
