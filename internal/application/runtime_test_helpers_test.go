package application

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func useApplicationScopedDataProviders(t *testing.T) {
	t.Helper()
	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication))
	t.Setenv(ProviderScopeEnv(capability.ProviderValkey), string(capability.ScopeApplication))
}
