package deployment

import (
	"fmt"
	"strings"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const RuntimeProviderEnvKey = "BASEHARBOR_RUNTIME_PROVIDER"

// RuntimeProviderState is deployment-owned provider selection. It is not part
// of the portable application contract and may differ between deployments of
// the same logical application.
type RuntimeProviderState struct {
	Provider bhruntime.ProviderKind
}

// RuntimeProviderStateFromValues reads protected deployment state. Missing
// provider metadata from v0.3 deployments intentionally defaults to Compose so
// existing installations remain compatible without a migration.
func RuntimeProviderStateFromValues(values map[string]string) (RuntimeProviderState, error) {
	kind, err := bhruntime.ParseProviderKind(values[RuntimeProviderEnvKey])
	if err != nil {
		return RuntimeProviderState{}, fmt.Errorf("runtime provider state: %w", err)
	}
	return RuntimeProviderState{Provider: kind}, nil
}

// ApplyRuntimeProviderState writes normalized deployment metadata into the
// supplied state map while preserving unrelated deployment-owned values.
// Provider-only legacy writers also materialize the standard runtime profile
// when no profile has been persisted yet, so protected deployment state is a
// complete provider+profile selection without requiring a migration step.
func ApplyRuntimeProviderState(values map[string]string, state RuntimeProviderState) error {
	if values == nil {
		return fmt.Errorf("runtime provider state target is nil")
	}
	kind, err := bhruntime.ParseProviderKind(string(state.Provider))
	if err != nil {
		return fmt.Errorf("runtime provider state: %w", err)
	}
	values[RuntimeProviderEnvKey] = strings.TrimSpace(string(kind))
	if strings.TrimSpace(values[RuntimeProfileEnvKey]) == "" {
		values[RuntimeProfileEnvKey] = string(RuntimeProfileStandard)
	}
	return nil
}
