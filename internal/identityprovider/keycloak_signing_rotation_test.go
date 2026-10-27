package identityprovider

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
)

func TestVerifyRS256JWTWithJWKSRejectsRetiredKid(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	token := signedTestJWT(t, privateKey, "old-kid")
	jwks := keycloakJWKS{Keys: []keycloakJWK{rsaJWK(t, &privateKey.PublicKey, "old-kid")}}
	if err := verifyRS256JWTWithJWKS(token, jwks); err != nil {
		t.Fatalf("verify published key: %v", err)
	}
	jwks.Keys = nil
	if err := verifyRS256JWTWithJWKS(token, jwks); err == nil || !strings.Contains(err.Error(), "does not contain") {
		t.Fatalf("retired kid unexpectedly verified: %v", err)
	}
}

func TestSigningJWKSOverlapContainsOldAndNew(t *testing.T) {
	oldKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	newKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := keycloakJWKS{Keys: []keycloakJWK{
		rsaJWK(t, &oldKey.PublicKey, "old"),
		rsaJWK(t, &newKey.PublicKey, "new"),
	}}
	oldToken := signedTestJWT(t, oldKey, "old")
	newToken := signedTestJWT(t, newKey, "new")
	if !jwksContainsKid(jwks, "old") || !jwksContainsKid(jwks, "new") {
		t.Fatal("JWKS overlap does not contain both kids")
	}
	if err := verifyRS256JWTWithJWKS(oldToken, jwks); err != nil {
		t.Fatalf("old token failed during overlap: %v", err)
	}
	if err := verifyRS256JWTWithJWKS(newToken, jwks); err != nil {
		t.Fatalf("new token failed during overlap: %v", err)
	}

	jwks.Keys = jwks.Keys[1:]
	if jwksContainsKid(jwks, "old") {
		t.Fatal("old kid remained after retirement")
	}
	if err := verifyRS256JWTWithJWKS(oldToken, jwks); err == nil {
		t.Fatal("old token still verified after retirement")
	}
	if err := verifyRS256JWTWithJWKS(newToken, jwks); err != nil {
		t.Fatalf("new token failed after retirement: %v", err)
	}
}

func TestManagedSigningComponentsIgnoreForeignProviders(t *testing.T) {
	components := []keycloakComponent{
		{ID: "foreign", Name: "default-rsa", ProviderID: keycloakRSAProviderID},
		{ID: "managed", Name: managedSigningPrefix + "initial", ProviderID: keycloakRSAProviderID},
		{ID: "managed-other", Name: managedSigningPrefix + "ec", ProviderID: "ecdsa-generated"},
	}
	got := managedSigningComponents(components)
	if len(got) != 1 || got[0].ID != "managed" {
		t.Fatalf("managedSigningComponents() = %#v", got)
	}
}

func signedTestJWT(t *testing.T, privateKey *rsa.PrivateKey, kid string) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": kid})
	payload, _ := json.Marshal(map[string]any{"sub": "rotation-probe", "iat": 1})
	h := base64.RawURLEncoding.EncodeToString(header)
	p := base64.RawURLEncoding.EncodeToString(payload)
	sum := sha256.Sum256([]byte(h + "." + p))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return h + "." + p + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func rsaJWK(t *testing.T, publicKey *rsa.PublicKey, kid string) keycloakJWK {
	t.Helper()
	e := big.NewInt(int64(publicKey.E)).Bytes()
	return keycloakJWK{
		Kty: "RSA",
		Kid: kid,
		Alg: "RS256",
		N:   base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(e),
	}
}
