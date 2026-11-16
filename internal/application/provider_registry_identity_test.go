package application

import "testing"

func TestReferenceProviderRegistryAcceptsManagedIdentity(t *testing.T) {
	m := New("demo", "dev", false, false, false)
	m.Services.SQL = false
	m.Services.Identity = true
	m.Identity.Scopes = []string{"openid", "profile"}

	if err := CheckReferenceProviderRegistryAt(t.TempDir(), m); err != nil {
		t.Fatalf("managed identity provider registry preflight failed: %v", err)
	}
}
