package application

import "testing"

// These tests pin the name→dev-mode seam replaced by card t-20261114-r28-cli2-env-policy (0428-053):
// the environment NAME no longer alone decides workload security development
// mode. The decision now follows an explicit devMode flag; the historical
// mapping is preserved by ResolveWorkloadSecurityPolicy so no existing guarded
// environment weakens.

// TestResolveWorkloadSecurityPolicyExplicitDevMode pins that an explicit
// development policy overrides the name and yields development mode.
func TestResolveWorkloadSecurityPolicyExplicitDevMode(t *testing.T) {
	policy, err := ResolveWorkloadSecurityPolicyWithDevPolicy(New("demo", "prod", false, false, true), true)
	if err != nil {
		t.Fatalf("ResolveWorkloadSecurityPolicyWithDevMode: %v", err)
	}
	if policy.Mode != "development" {
		t.Fatalf("explicit development policy must yield development mode, got %q", policy.Mode)
	}

	managed, err := ResolveWorkloadSecurityPolicyWithDevPolicy(New("demo", "prod", false, false, true), false)
	if err != nil {
		t.Fatalf("ResolveWorkloadSecurityPolicyWithDevMode(managed): %v", err)
	}
	if managed.Mode != "managed" {
		t.Fatalf("explicit managed policy must yield managed mode, got %q", managed.Mode)
	}
}

// TestResolveWorkloadSecurityPolicyPreservesNameMapping preserves the historical
// behavior of ResolveWorkloadSecurityPolicy(name).
func TestResolveWorkloadSecurityPolicyPreservesNameMapping(t *testing.T) {
	dev, err := ResolveWorkloadSecurityPolicy(New("demo", "dev", false, false, true))
	if err != nil {
		t.Fatalf("ResolveWorkloadSecurityPolicy(dev): %v", err)
	}
	if dev.Mode != "development" {
		t.Fatalf("dev must stay development mode via the preserved name mapping: %+v", dev)
	}

	prod, err := ResolveWorkloadSecurityPolicy(New("demo", "prod", false, false, true))
	if err != nil {
		t.Fatalf("ResolveWorkloadSecurityPolicy(prod): %v", err)
	}
	if prod.Mode != "managed" {
		t.Fatalf("prod must stay managed mode via the preserved name mapping: %+v", prod)
	}
}

// TestTrustedLocalForDevEnvironment pins the single visible name→dev mapping.
func TestTrustedLocalForDevEnvironment(t *testing.T) {
	for _, name := range []string{"dev", "development"} {
		if !TrustedLocalForDevEnvironment(name) {
			t.Fatalf("name %q must map to development mode", name)
		}
	}
	for _, name := range []string{"", "test", "staging", "prod", "production", "custom-env"} {
		if TrustedLocalForDevEnvironment(name) {
			t.Fatalf("name %q must map to managed", name)
		}
	}
}
