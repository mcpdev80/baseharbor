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
	for _, name := range []string{"dev", "development", "prod", "custom-env"} {
		policy, err := ResolveWorkloadSecurityPolicyWithDevPolicy(New("demo", name, false, false, true), true)
		if err != nil {
			t.Fatalf("ResolveWorkloadSecurityPolicyWithDevMode(%q): %v", name, err)
		}
		if policy.Mode != "development" {
			t.Fatalf("explicit development policy for %q must yield development mode, got %q", name, policy.Mode)
		}

		managed, err := ResolveWorkloadSecurityPolicyWithDevPolicy(New("demo", name, false, false, true), false)
		if err != nil {
			t.Fatalf("ResolveWorkloadSecurityPolicyWithDevMode(%q, managed): %v", name, err)
		}
		if managed.Mode != "managed" {
			t.Fatalf("explicit managed policy for %q must yield managed mode, got %q", name, managed.Mode)
		}
	}
}

// TestResolveWorkloadSecurityPolicyPreservesNameMapping preserves the historical
// TestResolveWorkloadSecurityPolicyCompatibilityWrapperFailsClosed proves
// environment names do not grant development-mode security relaxations.
func TestResolveWorkloadSecurityPolicyCompatibilityWrapperFailsClosed(t *testing.T) {
	for _, name := range []string{"dev", "development", "test", "staging", "prod", "custom-env"} {
		policy, err := ResolveWorkloadSecurityPolicy(New("demo", name, false, false, true))
		if err != nil {
			t.Fatalf("ResolveWorkloadSecurityPolicy(%q): %v", name, err)
		}
		if policy.Mode != "managed" {
			t.Fatalf("name %q must fail closed as managed: %+v", name, policy)
		}
		if TrustedLocalForDevEnvironment(name) {
			t.Fatalf("name %q must not establish development mode", name)
		}
	}
}

func TestUntrustedEnvironmentNameCannotEnableDevelopmentMode(t *testing.T) {
	t.Setenv(WorkloadSecurityModeEnv, "development")
	for _, name := range []string{"dev", "development", "prod", "custom-env"} {
		if _, err := ResolveWorkloadSecurityPolicy(New("demo", name, false, false, true)); err == nil {
			t.Fatalf("name-only policy accepted development mode for %q", name)
		}
	}
}
