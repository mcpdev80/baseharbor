package deployment

import (
	"fmt"
	"strings"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const RuntimeProfileEnvKey = "BASEHARBOR_RUNTIME_PROFILE"

type RuntimeProfile string

const (
	RuntimeProfileStandard RuntimeProfile = "standard"
)

// RuntimeSelection is deployment-owned runtime intent. The same portable
// application contract may use a different selection in each environment.
type RuntimeSelection struct {
	Provider bhruntime.ProviderKind
	Profile  RuntimeProfile
}

func ParseRuntimeProfile(value string) (RuntimeProfile, error) {
	profile := RuntimeProfile(strings.TrimSpace(strings.ToLower(value)))
	if profile == "" {
		profile = RuntimeProfileStandard
	}
	switch profile {
	case RuntimeProfileStandard:
		return profile, nil
	default:
		return "", fmt.Errorf("unsupported runtime profile %q", value)
	}
}

// RuntimeSelectionFromValues reads explicit deployment state. Environment names
// do not imply a provider or profile; selection stays explicit and independently
// configurable for development, staging, production, or future environments.
func RuntimeSelectionFromValues(values map[string]string) (RuntimeSelection, error) {
	providerState, err := RuntimeProviderStateFromValues(values)
	if err != nil {
		return RuntimeSelection{}, err
	}
	profile, err := ParseRuntimeProfile(values[RuntimeProfileEnvKey])
	if err != nil {
		return RuntimeSelection{}, fmt.Errorf("runtime selection state: %w", err)
	}
	return RuntimeSelection{Provider: providerState.Provider, Profile: profile}, nil
}

// ApplyRuntimeSelection writes normalized deployment-owned selection while
// preserving unrelated state such as hostname, TLS, and environment metadata.
func ApplyRuntimeSelection(values map[string]string, selection RuntimeSelection) error {
	if err := ApplyRuntimeProviderState(values, RuntimeProviderState{Provider: selection.Provider}); err != nil {
		return err
	}
	profile, err := ParseRuntimeProfile(string(selection.Profile))
	if err != nil {
		return fmt.Errorf("runtime selection state: %w", err)
	}
	values[RuntimeProfileEnvKey] = string(profile)
	return nil
}
