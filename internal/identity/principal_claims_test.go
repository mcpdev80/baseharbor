package identity

import (
	"reflect"
	"testing"
)

func TestPrincipalClaimSetsNamespacesGroupsAndRoles(t *testing.T) {
	p := &Principal{
		Groups:         []string{"team-ops", "team-dev"},
		AppRoles:       []string{"db.admin", "reader"},
		IssuerVerified: true,
	}
	got := p.ClaimSets()
	want := []string{"app-role:db.admin", "app-role:reader", "group:team-dev", "group:team-ops"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ClaimSets()=%v want %v", got, want)
	}
}

func TestPrincipalClaimSetsDeduplicatesAndCasesToLower(t *testing.T) {
	p := &Principal{
		Groups:         []string{"Ops", "ops", "  ", ""},
		AppRoles:       []string{"Admin"},
		IssuerVerified: true,
	}
	got := p.ClaimSets()
	want := []string{"app-role:admin", "group:ops"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ClaimSets()=%v want %v", got, want)
	}
}

func TestPrincipalClaimSetsNilAndEmpty(t *testing.T) {
	if got := (*Principal)(nil).ClaimSets(); got != nil {
		t.Fatalf("nil principal ClaimSets()=%v want nil", got)
	}
	empty := &Principal{IssuerVerified: true}
	if got := empty.ClaimSets(); got != nil {
		t.Fatalf("empty principal ClaimSets()=%v want nil", got)
	}
}
