package main

import "testing"

func TestResolveTargetDeploymentPolicyFlags(t *testing.T) {
	if got, err := resolveTargetDeploymentPolicyFlags("", "", nil); err != nil || got != nil {
		t.Fatalf("unconfigured policy must remain absent, got %+v / %v", got, err)
	}
	p, err := resolveTargetDeploymentPolicyFlags("test", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.AllowDirty || !p.RequireCommit || !p.RequireImageDigest || p.AllowsBranch("main") {
		t.Fatalf("incorrect test preset: %+v", p)
	}
	override, err := resolveTargetDeploymentPolicyFlags("production", "all", map[string]string{
		"--deployment-dirty":         "yes",
		"--deployment-commit":        "no",
		"--deployment-remote-commit": "no",
		"--deployment-approval":      "no",
		"--deployment-image-digest":  "no",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !override.AllowDirty || override.RequireCommit || override.RequireRemoteCommit ||
		override.RequireApproval || override.RequireImageDigest || !override.AllowsBranch("feature/not-committed") {
		t.Fatalf("operator overrides discarded: %+v", override)
	}
	if _, err := resolveTargetDeploymentPolicyFlags("production", "main,,release/*", nil); err == nil {
		t.Fatal("invalid branch selection accepted")
	}
	if _, err := resolveTargetDeploymentPolicyFlags("test", "", map[string]string{"--deployment-dirty": "maybe"}); err == nil {
		t.Fatal("invalid boolean accepted")
	}
}
