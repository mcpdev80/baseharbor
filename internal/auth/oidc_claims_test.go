package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

var _ = errors.Is

// signTestIDToken is defined in oidc_test.go in the same test package.
func TestOIDCVerifierExtractsVerifiedGroupsAndAppRoles(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	const issuer = "https://issuer.example"
	keySet := &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}
	verifier, err := newOIDCVerifier(issuer, []string{"baseharbor"}, func(audience string) *oidc.IDTokenVerifier {
		return oidc.NewVerifier(issuer, keySet, &oidc.Config{ClientID: audience})
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	token := signTestIDToken(t, key, map[string]any{
		"iss":      issuer,
		"sub":      "subject-1",
		"aud":      "baseharbor",
		"exp":      now.Add(time.Hour).Unix(),
		"iat":      now.Add(-time.Minute).Unix(),
		"group":    []string{"team-ops", "team-dev"},
		"app_role": []string{"db.admin"},
	})

	principal, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	if !principal.IssuerVerified {
		t.Fatalf("expected IssuerVerified=true for a verified token")
	}
	if len(principal.Groups) != 2 {
		t.Fatalf("Groups=%v want 2 entries", principal.Groups)
	}
	if len(principal.AppRoles) != 1 {
		t.Fatalf("AppRoles=%v want 1 entry", principal.AppRoles)
	}
}

func TestOIDCVerifierClaimSetsEmptyWithoutGroupClaims(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const issuer = "https://issuer.example"
	keySet := &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}
	verifier, err := newOIDCVerifier(issuer, []string{"baseharbor"}, func(audience string) *oidc.IDTokenVerifier {
		return oidc.NewVerifier(issuer, keySet, &oidc.Config{ClientID: audience})
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	token := signTestIDToken(t, key, map[string]any{
		"iss": issuer, "sub": "subject-2", "aud": "baseharbor",
		"exp": now.Add(time.Hour).Unix(),
	})
	principal, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	if principal.Issuer != issuer || principal.Subject != "subject-2" {
		t.Fatalf("unexpected principal: %#v", principal)
	}
	if len(principal.Groups) != 0 || len(principal.AppRoles) != 0 {
		t.Fatalf("expected no claim sets, got groups=%v roles=%v", principal.Groups, principal.AppRoles)
	}
}

func TestOIDCVerifierStringDoesNotExposeClaims(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const issuer = "https://issuer.example"
	keySet := &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}
	verifier, err := newOIDCVerifier(issuer, []string{"baseharbor"}, func(audience string) *oidc.IDTokenVerifier {
		return oidc.NewVerifier(issuer, keySet, &oidc.Config{ClientID: audience})
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	token := signTestIDToken(t, key, map[string]any{
		"iss": issuer, "sub": "subject-3", "aud": "baseharbor",
		"exp":   now.Add(time.Hour).Unix(),
		"group": []string{"super-secret-group"},
	})
	principal, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	s := principal.String()
	if containsAny(s, "super-secret-group") {
		t.Fatalf("principal string leaked claim group: %s", s)
	}
}

func containsAny(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

var _ = errors.Is
