package serviceaccess

import "testing"

// These tests pin the name→policy seam replaced by card t-20261114-r28-cli2-env-policy (0428-053):
// the environment NAME no longer reaches into the authentication decision. The
// decision now follows an explicit EnvironmentPolicy; the historical mapping is
// preserved through TrustedLocalForEnvironment so no existing guarded
// environment weakens.

// TestResolveNameNoLongerDrivesAuthentication proves a name cannot by itself
// decide authentication: two names with the same explicit policy behave
// identically, and an explicit trusted-local policy is what makes auth optional.
func TestResolveNameNoLongerDrivesAuthentication(t *testing.T) {
	devManaged := func(env string) bool {
		p, err := ResolveWithPolicy(env, "prometheus", AuthenticationMTLS, EnvironmentPolicy{TrustedLocal: true})
		if err != nil {
			t.Fatalf("ResolveWithPolicy(%q): %v", env, err)
		}
		return p.AuthenticationRequired
	}
	managed := func(env string) bool {
		p, err := ResolveWithPolicy(env, "prometheus", AuthenticationMTLS, EnvironmentPolicy{TrustedLocal: false})
		if err != nil {
			t.Fatalf("ResolveWithPolicy(%q): %v", env, err)
		}
		return p.AuthenticationRequired
	}

	for _, name := range []string{"dev", "development", "test", "staging", "prod", "custom-1"} {
		if devManaged(name) != devManaged("sibling") {
			t.Fatalf("name %q: trusted-local policy must behave the same as any other name", name)
		}
		if managed(name) != managed("sibling") {
			t.Fatalf("name %q: managed policy must behave the same as any other name", name)
		}
	}
}

// TestResolveExplicitTrustedLocalAllowsOptionalAuth pins that a trusted-local
// policy still permits authentication to be optional while TLS stays mandatory.
func TestResolveExplicitTrustedLocalAllowsOptionalAuth(t *testing.T) {
	p, err := ResolveWithPolicy("prod", "prometheus", AuthenticationNone, EnvironmentPolicy{TrustedLocal: true})
	if err != nil {
		t.Fatalf("ResolveWithPolicy: %v", err)
	}
	if p.AuthenticationRequired {
		t.Fatalf("explicit trusted-local policy must keep authentication optional: %+v", p)
	}
	if !p.TLSRequired {
		t.Fatalf("TLS must remain mandatory even for a trusted-local environment: %+v", p)
	}
}

// TestResolveDefaultPolicyIsManagedFailsClosed pins that callers which do not
// pass an explicit policy are treated as managed (auth required), so an
// unset/unknown policy never weakens guarantees.
func TestResolveDefaultPolicyIsManagedFailsClosed(t *testing.T) {
	p, err := ResolveWithPolicy("prod", "prometheus", AuthenticationMTLS, EnvironmentPolicy{})
	if err != nil {
		t.Fatalf("ResolveWithPolicy: %v", err)
	}
	if !p.AuthenticationRequired {
		t.Fatalf("empty policy (fail-closed default) must be managed/auth-required: %+v", p)
	}
}

// TestResolveByDefaultWrapsNameMapping preserves the historical behavior that
// Resolve(name) treats dev/development as trusted-local and everything else as
// managed.
func TestResolveByDefaultWrapsNameMapping(t *testing.T) {
	dev, err := Resolve("dev", "prometheus", AuthenticationMTLS)
	if err != nil {
		t.Fatalf("Resolve(dev): %v", err)
	}
	if dev.AuthenticationRequired {
		t.Fatalf("dev must stay trusted-local via the preserved name mapping: %+v", dev)
	}

	prod, err := Resolve("prod", "prometheus", AuthenticationMTLS)
	if err != nil {
		t.Fatalf("Resolve(prod): %v", err)
	}
	if !prod.AuthenticationRequired {
		t.Fatalf("prod must stay managed via the preserved name mapping: %+v", prod)
	}

	custom, err := Resolve("staging", "prometheus", AuthenticationMTLS)
	if err != nil {
		t.Fatalf("Resolve(staging): %v", err)
	}
	if !custom.AuthenticationRequired {
		t.Fatalf("custom environment must be managed via the preserved name mapping: %+v", custom)
	}
}

// TestTrustedLocalForEnvironment pins the single visible name→trusted mapping.
func TestTrustedLocalForEnvironment(t *testing.T) {
	for _, name := range []string{"dev", "development"} {
		if !TrustedLocalForEnvironment(name) {
			t.Fatalf("name %q must map to trusted-local", name)
		}
	}
	for _, name := range []string{"", "test", "staging", "prod", "production", "custom-env"} {
		if TrustedLocalForEnvironment(name) {
			t.Fatalf("name %q must map to managed (not trusted-local)", name)
		}
	}
}
