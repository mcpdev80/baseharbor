package operatorauth

import "testing"

// These tests pin the name→managed seam replaced by card t-20261114-r28-cli2-env-policy (0428-053):
// the environment NAME no longer alone decides managed vs trusted-local. The
// decision now follows an explicit trustedLocal flag; the historical mapping is
// preserved by ManagedEnvironment so no existing guarded environment weakens.

// TestManagedEnvironmentWithPolicyExplicitTrustedLocal pins that an explicit
// trusted-local policy overrides the name and makes the environment unmanaged.
func TestManagedEnvironmentWithPolicyExplicitTrustedLocal(t *testing.T) {
	if ManagedEnvironmentWithPolicy("prod", true) {
		t.Fatal("explicit trusted-local policy must make even prod unmanaged")
	}
	if ManagedEnvironmentWithPolicy("custom-1", true) {
		t.Fatal("explicit trusted-local policy must make any environment unmanaged")
	}
}

// TestManagedEnvironmentWithPolicyFailsClosed pins that an unset policy (false)
// keeps the historical name mapping and never treats an environment as
// unmanaged by mistake.
func TestManagedEnvironmentWithPolicyFailsClosed(t *testing.T) {
	if ManagedEnvironmentWithPolicy("prod", false) != true {
		t.Fatal("managed env (prod, no explicit policy) must be managed")
	}
	if ManagedEnvironmentWithPolicy("custom-1", false) != true {
		t.Fatal("managed env (custom, no explicit policy) must be managed")
	}
}

// TestManagedEnvironmentPreservesNameMapping preserves the historical
// behavior of the single-argument ManagedEnvironment(name).
func TestManagedEnvironmentPreservesNameMapping(t *testing.T) {
	if ManagedEnvironment("dev") || ManagedEnvironment("development") {
		t.Fatal("dev/development must stay trusted-local (unmanaged)")
	}
	if !ManagedEnvironment("test") || !ManagedEnvironment("prod") {
		t.Fatal("test/prod must require managed operator authentication")
	}
	if !ManagedEnvironment("custom-env") {
		t.Fatal("custom environment must be managed")
	}
}
