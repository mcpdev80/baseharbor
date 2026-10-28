package credential

import (
	"fmt"
	"strings"
)

// Class is the normative BaseHarbor credential ownership class.
type Class string

const (
	ClassHumanManagement    Class = "human-management"
	ClassApplicationService Class = "application-service"
	ClassInternalMachine    Class = "internal-machine"
)

// ParseClass validates the public v1 credential taxonomy.
func ParseClass(value string) (Class, error) {
	class := Class(strings.TrimSpace(value))
	switch class {
	case ClassHumanManagement, ClassApplicationService, ClassInternalMachine:
		return class, nil
	default:
		return "", fmt.Errorf("unsupported credential class %q", value)
	}
}

// Shareable reports whether the class may participate in an explicit sharing
// policy. Internal machine identity is never shareable.
func (c Class) Shareable() bool {
	return c == ClassHumanManagement || c == ClassApplicationService
}

// IsMachineIdentity reports whether the class belongs to the isolated
// BaseHarbor-internal machine identity boundary.
func (c Class) IsMachineIdentity() bool {
	return c == ClassInternalMachine
}
