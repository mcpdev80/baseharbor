package workstationenrollment

import (
	"context"
	"testing"
	"time"
)

// expiryRejected proves the one-use store rejects an already-expired grant: the
// expiry guard in the consume path is covered by a direct store test.
func TestExpiryRejected(t *testing.T) {
	store := NewMemoryStore()
	scope := Scope{TenantID: "11111111-1111-4111-8111-111111111111", Station: "dev-laptop-01"}
	if err := scope.Validate(); err != nil {
		t.Fatal(err)
	}
	bootstrap := Bootstrap{
		Token:          "tok",
		Nonce:          "non",
		ExpiresAt:      time.Now().Add(-time.Minute), // already elapsed
		CertificateTTL: time.Hour,
		RequestScope:   scope,
	}
	if err := store.Create(context.Background(), bootstrap); err != nil {
		t.Fatalf("store.Create accepted an expired bootstrap: %v", err)
	}
	if _, err := store.Consume(context.Background(), scope, "tok", "non", time.Hour); err == nil {
		t.Fatal("an expired one-use bootstrap must fail closed")
	}
}
