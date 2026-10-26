package tenancy

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestResolveSingleTenantAggregatesRoles(t *testing.T) {
	resolved, err := Resolve("identity-1", []Membership{
		{TenantID: "tenant-a", Role: "viewer"},
		{TenantID: "tenant-a", Role: "editor"},
		{TenantID: "tenant-a", Role: "viewer"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.TenantID != "tenant-a" {
		t.Fatalf("unexpected tenant: %s", resolved.TenantID)
	}
	if !reflect.DeepEqual(resolved.Roles, []string{"editor", "viewer"}) {
		t.Fatalf("unexpected roles: %#v", resolved.Roles)
	}
}

func TestResolveDifferentTenantsFailsClosed(t *testing.T) {
	_, err := Resolve("identity-1", []Membership{
		{TenantID: "tenant-a", Role: "viewer"},
		{TenantID: "tenant-b", Role: "viewer"},
	})
	if !errors.Is(err, ErrAmbiguousTenant) {
		t.Fatalf("expected ErrAmbiguousTenant, got %v", err)
	}
}

func TestResolveNoMembershipFailsClosed(t *testing.T) {
	_, err := Resolve("identity-1", nil)
	if !errors.Is(err, ErrNoMembership) {
		t.Fatalf("expected ErrNoMembership, got %v", err)
	}
}

func TestTenantContextRoundTrip(t *testing.T) {
	tenant := &Context{TenantID: "tenant-a", ExternalIdentityID: "identity-1", Roles: []string{"viewer"}}
	ctx := WithContext(context.Background(), tenant)
	got, ok := FromContext(ctx)
	if !ok || got.TenantID != tenant.TenantID {
		t.Fatalf("unexpected tenant context: %#v", got)
	}
}
