package serviceaccess

import "testing"

// These tests pin the name→policy seam replaced by card t-20261114-r28-cli2-env-policy (0428-053):
// the environment NAME no longer reaches into the authentication decision. The
// decision now follows an explicit EnvironmentPolicy; compatibility wrappers
// with no policy source fail closed for every name.

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

// TestResolveCompatibilityWrapperFailsClosedForEveryName pins that the legacy
// API cannot grant trusted-local behavior based on an environment name.
func TestResolveCompatibilityWrapperFailsClosedForEveryName(t *testing.T) {
	for _, name := range []string{"dev", "development", "test", "staging", "prod", "production", "custom-env"} {
		p, err := Resolve(name, "prometheus", AuthenticationMTLS)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", name, err)
		}
		if !p.AuthenticationRequired {
			t.Fatalf("Resolve(%q) must fail closed as managed: %+v", name, p)
		}
		if TrustedLocalForEnvironment(name) {
			t.Fatalf("name %q must not establish trusted-local policy", name)
		}
	}
}
