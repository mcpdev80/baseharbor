package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

func TestOIDCVerifierAcceptsConfiguredAudienceAndRejectsInvalidTokens(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	const issuer = "https://issuer.example"
	keySet := &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}
	verifier, err := newOIDCVerifier(issuer, []string{"baseharbor", "baseharbor-cli"}, func(audience string) *oidc.IDTokenVerifier {
		return oidc.NewVerifier(issuer, keySet, &oidc.Config{ClientID: audience})
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	valid := signTestIDToken(t, key, map[string]any{
		"iss": issuer,
		"sub": "subject-1",
		"aud": "baseharbor-cli",
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Add(-time.Minute).Unix(),
	})
	principal, err := verifier.Verify(context.Background(), valid)
	if err != nil {
		t.Fatalf("verify valid token: %v", err)
	}
	if principal.Issuer != issuer || principal.Subject != "subject-1" {
		t.Fatalf("unexpected principal: %#v", principal)
	}
	if len(principal.Audience) != 1 || principal.Audience[0] != "baseharbor-cli" {
		t.Fatalf("unexpected audience: %#v", principal.Audience)
	}

	wrongAudience := signTestIDToken(t, key, map[string]any{
		"iss": issuer,
		"sub": "subject-1",
		"aud": "other-client",
		"exp": now.Add(time.Hour).Unix(),
	})
	if _, err := verifier.Verify(context.Background(), wrongAudience); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("wrong audience error = %v, want ErrInvalidToken", err)
	}

	expired := signTestIDToken(t, key, map[string]any{
		"iss": issuer,
		"sub": "subject-1",
		"aud": "baseharbor",
		"exp": now.Add(-time.Hour).Unix(),
	})
	if _, err := verifier.Verify(context.Background(), expired); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired token error = %v, want ErrInvalidToken", err)
	}

	wrongIssuer := signTestIDToken(t, key, map[string]any{
		"iss": "https://attacker.example",
		"sub": "subject-1",
		"aud": "baseharbor",
		"exp": now.Add(time.Hour).Unix(),
	})
	if _, err := verifier.Verify(context.Background(), wrongIssuer); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("wrong issuer error = %v, want ErrInvalidToken", err)
	}
}

func TestOIDCVerifierRejectsEmptyToken(t *testing.T) {
	verifier := &OIDCVerifier{}
	if _, err := verifier.Verify(context.Background(), ""); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("empty token error = %v, want ErrInvalidToken", err)
	}
}

func signTestIDToken(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	header, err := json.Marshal(map[string]any{"alg": "RS256", "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	encode := base64.RawURLEncoding.EncodeToString
	signingInput := encode(header) + "." + encode(payload)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + encode(signature)
}
