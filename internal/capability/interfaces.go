package capability

import (
	"fmt"
	"strings"
)

type ProviderInterfaceClass string

const (
	InterfaceApplication    ProviderInterfaceClass = "application"
	InterfaceUserFacing     ProviderInterfaceClass = "user-facing"
	InterfaceAdministration ProviderInterfaceClass = "administration"
	InterfaceManagement     ProviderInterfaceClass = "management"
	InterfaceObservability  ProviderInterfaceClass = "observability"
	InterfaceHealth         ProviderInterfaceClass = "health"
)

type ProviderInterface struct {
	Name      string                 `json:"name"`
	Class     ProviderInterfaceClass `json:"class"`
	Protocol  string                 `json:"protocol,omitempty"`
	Optional  bool                   `json:"optional,omitempty"`
	Intrinsic bool                   `json:"intrinsic,omitempty"`
	Path      string                 `json:"path,omitempty"`
}

func (i ProviderInterface) Validate() error {
	if strings.TrimSpace(i.Name) == "" {
		return fmt.Errorf("provider interface name is required")
	}
	switch i.Class {
	case InterfaceApplication, InterfaceUserFacing, InterfaceAdministration, InterfaceManagement, InterfaceObservability, InterfaceHealth:
	default:
		return fmt.Errorf("provider interface %q has unsupported class %q", i.Name, i.Class)
	}
	if i.Optional && i.Intrinsic {
		return fmt.Errorf("provider interface %q cannot be both optional and intrinsic", i.Name)
	}
	if i.Path != "" && !strings.HasPrefix(i.Path, "/") {
		return fmt.Errorf("provider interface %q path must be absolute", i.Name)
	}
	return nil
}
