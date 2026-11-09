package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestExistingTargetPolicyPreservesOverridesAndCoreBinding(t *testing.T) {
	target := configureTestTarget(t)
	ctx := context.Background()
	original, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	binding := original.Targets[target.Name]
	var out bytes.Buffer
	if err := setTargetDeploymentPolicy(ctx, []string{
		target.Name, "--deployment-preset", "test",
	}, &out, &out); err != nil {
		t.Fatalf("set test policy: %v", err)
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	policy := cfg.Targets[target.Name].DeploymentPolicy
	if policy == nil || policy.AllowDirty || !policy.RequireCommit || !policy.RequireImageDigest {
		t.Fatalf("test policy not persisted: %+v", policy)
	}
	out.Reset()
	if err := setTargetDeploymentPolicy(ctx, []string{
		target.Name, "--deployment-dirty", "yes",
	}, &out, &out); err != nil {
		t.Fatalf("customize existing policy: %v", err)
	}
	cfg, err = deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	updated := cfg.Targets[target.Name]
	if updated.DeploymentPolicy == nil || !updated.DeploymentPolicy.AllowDirty ||
		!updated.DeploymentPolicy.RequireCommit || !updated.DeploymentPolicy.RequireImageDigest {
		t.Fatalf("operator edit reset unrelated policy fields: %+v", updated.DeploymentPolicy)
	}
	if updated.TenantID != binding.TenantID || updated.Runtime != binding.Runtime ||
		updated.Access != binding.Access || updated.Scope != binding.Scope {
		t.Fatal("policy-only mutation changed Target Core or Runtime binding")
	}
	out.Reset()
	if err := showTargetDeploymentPolicy(ctx, []string{target.Name}, &out, &out); err != nil {
		t.Fatalf("show persisted policy: %v", err)
	}
	if !strings.Contains(out.String(), "require_commit") {
		t.Fatalf("persisted policy not visible: %s", out.String())
	}
}

func TestExistingTargetPolicyInvalidEditCannotMutate(t *testing.T) {
	target := configureTestTarget(t)
	ctx := context.Background()
	var out bytes.Buffer
	if err := setTargetDeploymentPolicy(ctx, []string{
		target.Name, "--deployment-preset", "test",
	}, &out, &out); err != nil {
		t.Fatal(err)
	}
	before, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := setTargetDeploymentPolicy(ctx, []string{
		target.Name, "--deployment-branches", "main,,release/*",
	}, &out, &out); err == nil {
		t.Fatal("invalid branch pattern accepted")
	}
	after, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(before.Targets[target.Name].DeploymentPolicy.AllowedBranches, ",") !=
		strings.Join(after.Targets[target.Name].DeploymentPolicy.AllowedBranches, ",") {
		t.Fatal("rejected policy edit changed existing branch restrictions")
	}
}
