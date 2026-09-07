package identity

import (
	"context"
	"strings"
	"testing"
)

func TestPrincipalContextRoundTrip(t *testing.T) {
	principal := &Principal{Issuer: "https://issuer.example", Subject: "user-123", Audience: []string{"baseharbor"}}
	ctx := WithPrincipal(context.Background(), principal)

	got, ok := FromContext(ctx)
	if !ok {
		t.Fatal("expected principal in context")
	}
	if got.Issuer != principal.Issuer || got.Subject != principal.Subject {
		t.Fatalf("unexpected principal: %#v", got)
	}
}

func TestPrincipalStringDoesNotExposeAudience(t *testing.T) {
	principal := &Principal{Issuer: "issuer", Subject: "subject", Audience: []string{"secret-ish-audience"}}
	got := principal.String()
	if strings.Contains(got, "secret-ish-audience") {
		t.Fatalf("logging representation leaked audience: %s", got)
	}
}

func TestMissingPrincipalFailsClosed(t *testing.T) {
	if principal, ok := FromContext(context.Background()); ok || principal != nil {
		t.Fatal("expected no principal")
	}
}
