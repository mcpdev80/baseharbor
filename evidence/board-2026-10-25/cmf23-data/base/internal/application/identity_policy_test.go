package application

import "testing"

func TestIdentityPolicyEnvironmentCannotWeakenApplicationMFA(t *testing.T) {
	m := WithIdentityAuthentication(New("demo", "prod", false, false, false), "required", []string{"passkey", "totp"}, false)
	t.Setenv(IdentityMFAEnv, "optional")
	if _, err := ResolveIdentityPolicy(m); err == nil {
		t.Fatal("environment weakened required application MFA")
	}
}

func TestIdentityPolicyEnvironmentMayStrengthenAndNarrowMethods(t *testing.T) {
	m := WithIdentityAuthentication(New("demo", "prod", false, false, false), "optional", []string{"passkey", "totp"}, false)
	t.Setenv(IdentityMFAEnv, "required")
	t.Setenv(IdentityMethodsEnv, "passkey")
	policy, err := ResolveIdentityPolicy(m)
	if err != nil {
		t.Fatal(err)
	}
	if policy.MFA != "required" || len(policy.Methods) != 1 || policy.Methods[0] != "passkey" {
		t.Fatalf("policy=%#v", policy)
	}
}

func TestIdentityPolicyRejectsPasswordlessWithoutWebAuthnMethod(t *testing.T) {
	m := WithIdentityAuthentication(New("demo", "prod", false, false, false), "required", []string{"totp"}, true)
	if _, err := ResolveIdentityPolicy(m); err == nil {
		t.Fatal("passwordless policy without passkey/webauthn must fail")
	}
}
