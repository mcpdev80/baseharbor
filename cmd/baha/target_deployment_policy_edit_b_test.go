package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestRemoteTargetPolicyEditRefusesLocalSubstitution(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg := deployment.Config{
		Version: deployment.ConfigVersion,
		Access: map[string]deployment.AccessDefinition{
			"connector": {Provider: "baseharbor-node-connector", Reference: "node-owned"},
		},
		Targets: map[string]deployment.TargetDefinition{
			"remote-prod": {
				TenantID: "11111111-1111-4111-8111-111111111111",
				Runtime:  deployment.RuntimeDefinition{Provider: "podman"},
				Access:   deployment.TargetAccess{Reference: "connector"},
			},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	// Editing a remote-access Target's policy from a workstation must be refused
	// with no local policy substitution, per the core operator mutation rule.
	err := setTargetDeploymentPolicy(context.Background(), []string{
		"remote-prod", "--deployment-preset", "test",
	}, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "owning Core operator") {
		t.Fatalf("remote policy edit accepted locally: %v", err)
	}
	after, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if after.Targets["remote-prod"].DeploymentPolicy != nil {
		t.Fatal("rejected remote edit changed persisted policy")
	}
}

func TestLocalTargetPolicyEditPreservesCoreBinding(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg := deployment.Config{
		Version: deployment.ConfigVersion,
		Access: map[string]deployment.AccessDefinition{
			"local-docker": {Provider: "local", Reference: "local"},
		},
		Targets: map[string]deployment.TargetDefinition{
			"docker-dev": {
				TenantID: "22222222-2222-4222-8222-222222222222",
				Runtime:  deployment.RuntimeDefinition{Provider: "docker"},
				Access:   deployment.TargetAccess{Reference: "local-docker"},
			},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	original := cfg.Targets["docker-dev"]
	var out bytes.Buffer
	if err := setTargetDeploymentPolicy(context.Background(), []string{
		"docker-dev", "--deployment-preset", "production",
	}, &out, &out); err != nil {
		t.Fatalf("set production policy: %v", err)
	}
	updated, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	policy := updated.Targets["docker-dev"].DeploymentPolicy
	if policy == nil || !policy.RequireApproval || !policy.RequireImageDigest {
		t.Fatalf("production policy not persisted: %+v", policy)
	}
	if updated.Targets["docker-dev"].TenantID != original.TenantID ||
		updated.Targets["docker-dev"].Runtime != original.Runtime ||
		updated.Targets["docker-dev"].Access != original.Access {
		t.Fatal("policy-only mutation changed Target tenant/runtime/access binding")
	}
}
