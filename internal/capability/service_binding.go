package capability

import (
	"fmt"
	"strings"
)

// Service Binding Specification 1.1 well-known entry names. BaseHarbor uses
// these names whenever the semantics match instead of defining aliases.
const (
	ServiceBindingType         = "type"
	ServiceBindingProvider     = "provider"
	ServiceBindingHost         = "host"
	ServiceBindingPort         = "port"
	ServiceBindingURI          = "uri"
	ServiceBindingUsername     = "username"
	ServiceBindingPassword     = "password"
	ServiceBindingCertificates = "certificates"
	ServiceBindingPrivateKey   = "private-key"
)

var serviceBindingWellKnownNames = map[string]struct{}{
	ServiceBindingType: {}, ServiceBindingProvider: {}, ServiceBindingHost: {},
	ServiceBindingPort: {}, ServiceBindingURI: {}, ServiceBindingUsername: {},
	ServiceBindingPassword: {}, ServiceBindingCertificates: {}, ServiceBindingPrivateKey: {},
}

// ValidateServiceBindingOutputName keeps standard connection names canonical.
// BaseHarbor-specific output names must use the explicit "baha." extension
// namespace so they cannot be confused with Service Binding entries.
func ValidateServiceBindingOutputName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("service binding output name is required")
	}
	if _, ok := serviceBindingWellKnownNames[name]; ok {
		return nil
	}
	if strings.HasPrefix(name, "baha.") && len(name) > len("baha.") {
		return nil
	}
	return fmt.Errorf("service binding output %q is neither a Service Binding 1.1 well-known name nor a versioned BaseHarbor extension", name)
}
