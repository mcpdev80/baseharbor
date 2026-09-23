package capability

import (
	"fmt"
	"strings"
)

// Service Binding Specification 1.1 well-known entry names.
// Use these names whenever the standard semantics match.
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
	ServiceBindingType:         {},
	ServiceBindingProvider:     {},
	ServiceBindingHost:         {},
	ServiceBindingPort:         {},
	ServiceBindingURI:          {},
	ServiceBindingUsername:     {},
	ServiceBindingPassword:     {},
	ServiceBindingCertificates: {},
	ServiceBindingPrivateKey:   {},
}

func IsServiceBindingWellKnownName(name string) bool {
	_, ok := serviceBindingWellKnownNames[strings.TrimSpace(name)]
	return ok
}

// ValidateServiceBindingName prevents BaseHarbor from creating aliases for
// Service Binding 1.1 well-known fields. Provider-specific or capability-specific
// extensions remain allowed when they use a namespaced name.
func ValidateServiceBindingName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("service binding entry name is required")
	}
	if IsServiceBindingWellKnownName(name) {
		return nil
	}
	switch strings.ToLower(name) {
	case "hostname", "connectionhost", "connection_host":
		return fmt.Errorf("service binding entry %q must use the standard name %q", name, ServiceBindingHost)
	case "user":
		return fmt.Errorf("service binding entry %q must use the standard name %q", name, ServiceBindingUsername)
	case "pass":
		return fmt.Errorf("service binding entry %q must use the standard name %q", name, ServiceBindingPassword)
	case "connectionstring", "connection_string":
		return fmt.Errorf("service binding entry %q must use the standard name %q", name, ServiceBindingURI)
	}
	if !strings.Contains(name, ".") && !strings.Contains(name, "/") {
		return fmt.Errorf("non-standard service binding entry %q must be namespaced", name)
	}
	return nil
}
