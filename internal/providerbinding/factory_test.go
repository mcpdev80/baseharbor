package providerbinding

import (
	"context"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/providerupgrade"
	"github.com/mcpdev80/baseharbor/internal/providerupgrade/openbao"
)

type noopInventory struct{}

func (noopInventory) InspectManaged(context.Context, providerupgrade.Provider) (RuntimeIdentity, error) {
	return RuntimeIdentity{}, errors.New("no runtime")
}
func TestNewForUnsupportedAndMissingDependencies(t *testing.T) {
	for _, provider := range []providerupgrade.Provider{"unknown", providerupgrade.ProviderOpenBao, providerupgrade.ProviderKeycloak} {
		if _, err := NewFor(provider, Dependencies{Inventory: noopInventory{}}); err == nil {
			t.Fatalf("accepted absent provider dependencies: %s", provider)
		}
	}
}
func TestNewForOpenBaoDoesNotRequireKeycloak(t *testing.T) {
	// OpenBao-specific hooks can be validated independently; a missing
	// Keycloak Core installation must never become an OpenBao prerequisite.
	deps := Dependencies{Inventory: noopInventory{}}
	_, err := NewFor(providerupgrade.ProviderOpenBao, deps)
	if err == nil {
		t.Fatal("incomplete OpenBao allowed")
	}
	if providerupgrade.ClassOf(err) != providerupgrade.ErrorDependency {
		t.Fatalf("unexpected class: %v", err)
	}
	// Standalone hook contract is provider-owned, never a SQL migration fallback.
	_ = openbao.RuntimeHooks{}
}
