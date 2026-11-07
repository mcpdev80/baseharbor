package database

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mcpdev80/baseharbor/internal/identity"
)

func testInitialBootstrapPrincipal(subject string) *identity.Principal {
	expiry := time.Now().Add(time.Hour)
	return &identity.Principal{
		Issuer:         "https://identity.example.test/realms/baseharbor",
		Subject:        subject,
		IssuerVerified: true,
		ExpiresAt:      &expiry,
	}
}

func TestValidateInitialOperatorBootstrap(t *testing.T) {
	now := time.Now()
	valid := testInitialBootstrapPrincipal("operator-owner")
	if err := ValidateInitialOperatorBootstrap(valid, "default", "Default", now); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		principal  *identity.Principal
		slug       string
		tenantName string
	}{
		{"missing principal", nil, "default", "Default"},
		{"unverified issuer", &identity.Principal{Issuer: valid.Issuer, Subject: valid.Subject, ExpiresAt: valid.ExpiresAt}, "default", "Default"},
		{"insecure issuer", &identity.Principal{Issuer: "http://identity.example", Subject: valid.Subject, IssuerVerified: true, ExpiresAt: valid.ExpiresAt}, "default", "Default"},
		{"expired token", &identity.Principal{Issuer: valid.Issuer, Subject: valid.Subject, IssuerVerified: true}, "default", "Default"},
		{"invalid slug", valid, "Default", "Default"},
		{"empty name", valid, "default", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateInitialOperatorBootstrap(tt.principal, tt.slug, tt.tenantName, now); err == nil {
				t.Fatal("invalid initial operator bootstrap was accepted")
			}
		})
	}
}

func TestBootstrapInitialOperatorTenantIntegration(t *testing.T) {
	dsn := os.Getenv("BASEHARBOR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires dedicated BASEHARBOR_TEST_DATABASE_URL")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	// Only the explicitly designated disposable test database is reset.
	if err := resetTestDatabase(ctx, admin); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, admin); err != nil {
		t.Fatal(err)
	}
	owner := testInitialBootstrapPrincipal("operator-a")
	first, err := BootstrapInitialOperatorTenant(ctx, admin, owner, "default", "Default")
	if err != nil || !first.Created || first.TenantID == "" {
		t.Fatalf("first bootstrap: %+v, %v", first, err)
	}
	retry, err := BootstrapInitialOperatorTenant(ctx, admin, owner, "default", "Default")
	if err != nil || retry.Created || retry.TenantID != first.TenantID || retry.ExternalIdentityID != first.ExternalIdentityID {
		t.Fatalf("idempotent retry: %+v, %v", retry, err)
	}
	if _, err := BootstrapInitialOperatorTenant(ctx, admin, testInitialBootstrapPrincipal("attacker"), "default", "Default"); !errors.Is(err, ErrInitialTenantOwnerMismatch) {
		t.Fatalf("wrong owner must be refused, got %v", err)
	}
	if _, err := BootstrapInitialOperatorTenant(ctx, admin, owner, "other", "Other"); !errors.Is(err, ErrInitialTenantExists) {
		t.Fatalf("second initial tenant must be refused, got %v", err)
	}
	resolved, err := NewIdentityTenantResolver(admin).ResolveTenant(ctx, owner)
	if err != nil || resolved.TenantID != first.TenantID {
		t.Fatalf("verified owner membership does not resolve: %#v, %v", resolved, err)
	}
}
