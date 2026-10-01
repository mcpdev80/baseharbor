package applicationbackup

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type RecoveryStateClass string

const (
	StateApplicationMetadata RecoveryStateClass = "application.metadata"
	StateSecrets             RecoveryStateClass = "secrets"
	StateSQL                 RecoveryStateClass = "database.sql"
	StateDurableKeyValue     RecoveryStateClass = "database.key-value"
	StateDocumentDatabase    RecoveryStateClass = "database.document"
	StateObjectStorage       RecoveryStateClass = "object-storage.s3"
	StateWorkloadStorage     RecoveryStateClass = "workload.storage"
	StateLogs                RecoveryStateClass = "observability.logs"
	StateMetrics             RecoveryStateClass = "observability.metrics"
	StateTraces              RecoveryStateClass = "observability.traces"
	StatePKI                 RecoveryStateClass = "security.pki"
	StateIdentity            RecoveryStateClass = "identity.oidc"
)

type RecoverySupport string

const (
	RecoverySupported   RecoverySupport = "supported"
	RecoveryUnsupported RecoverySupport = "unsupported"
	RecoveryExternal    RecoverySupport = "external"
)

type RecoveryContributor struct {
	StateClass         RecoveryStateClass `json:"state_class"`
	LogicalResource    string             `json:"logical_resource,omitempty"`
	Ownership          string             `json:"ownership"`
	Support            RecoverySupport    `json:"support"`
	DefaultSelected    bool               `json:"default_selected"`
	Selected           bool               `json:"selected"`
	Durable            bool               `json:"durable,omitempty"`
	ExplicitlyExcluded bool               `json:"explicitly_excluded,omitempty"`
	Reason             string             `json:"reason,omitempty"`
}

type RecoverySelection struct {
	Contributors []RecoveryContributor `json:"contributors"`
}

func NewRecoverySelection(contributors []RecoveryContributor) (RecoverySelection, error) {
	out := RecoverySelection{Contributors: append([]RecoveryContributor(nil), contributors...)}
	seen := make(map[string]struct{}, len(out.Contributors))
	for i := range out.Contributors {
		c := &out.Contributors[i]
		if err := c.Validate(); err != nil {
			return RecoverySelection{}, err
		}
		key := string(c.StateClass) + "\x00" + c.LogicalResource
		if _, ok := seen[key]; ok {
			return RecoverySelection{}, fmt.Errorf("duplicate recovery contributor %s %q", c.StateClass, c.LogicalResource)
		}
		seen[key] = struct{}{}
		c.Selected = c.DefaultSelected && c.Support == RecoverySupported
	}
	sort.SliceStable(out.Contributors, func(i, j int) bool {
		if out.Contributors[i].StateClass == out.Contributors[j].StateClass {
			return out.Contributors[i].LogicalResource < out.Contributors[j].LogicalResource
		}
		return out.Contributors[i].StateClass < out.Contributors[j].StateClass
	})
	return out, nil
}

func (c RecoveryContributor) Validate() error {
	switch c.StateClass {
	case StateApplicationMetadata, StateSecrets, StateSQL, StateDurableKeyValue, StateDocumentDatabase, StateObjectStorage, StateWorkloadStorage, StateLogs, StateMetrics, StateTraces, StatePKI, StateIdentity:
	default:
		return fmt.Errorf("unsupported recovery state class %q", c.StateClass)
	}
	switch c.Support {
	case RecoverySupported, RecoveryUnsupported, RecoveryExternal:
	default:
		return fmt.Errorf("invalid recovery support %q", c.Support)
	}
	if strings.TrimSpace(c.Ownership) == "" {
		return errors.New("recovery contributor ownership is required")
	}
	if c.StateClass != StateApplicationMetadata && strings.TrimSpace(c.LogicalResource) == "" {
		return fmt.Errorf("recovery contributor %s requires a logical resource", c.StateClass)
	}
	if c.Support != RecoverySupported && c.DefaultSelected {
		return fmt.Errorf("unsupported or external recovery contributor %s %q cannot be selected by default", c.StateClass, c.LogicalResource)
	}
	return nil
}

func (s RecoverySelection) Apply(include, exclude []RecoveryStateClass) (RecoverySelection, error) {
	out := RecoverySelection{Contributors: append([]RecoveryContributor(nil), s.Contributors...)}
	includes, err := recoveryClassSet(include)
	if err != nil {
		return RecoverySelection{}, err
	}
	excludes, err := recoveryClassSet(exclude)
	if err != nil {
		return RecoverySelection{}, err
	}
	for class := range includes {
		if _, blocked := excludes[class]; blocked {
			return RecoverySelection{}, fmt.Errorf("recovery state class %q cannot be both included and excluded", class)
		}
	}

	known := make(map[RecoveryStateClass]bool)
	supported := make(map[RecoveryStateClass]bool)
	for i := range out.Contributors {
		c := &out.Contributors[i]
		known[c.StateClass] = true
		if c.Support == RecoverySupported {
			supported[c.StateClass] = true
		}
		if _, ok := excludes[c.StateClass]; ok {
			if c.StateClass == StateApplicationMetadata {
				return RecoverySelection{}, errors.New("application.metadata is required in every recovery unit")
			}
			c.Selected = false
			c.ExplicitlyExcluded = true
		}
		if _, ok := includes[c.StateClass]; ok {
			if c.Support == RecoverySupported {
				c.Selected = true
				c.ExplicitlyExcluded = false
			}
		}
	}
	for class := range includes {
		if !known[class] {
			return RecoverySelection{}, fmt.Errorf("recovery state class %q is not present for this application", class)
		}
		if !supported[class] {
			return RecoverySelection{}, fmt.Errorf("recovery state class %q has no supported application-owned contributor", class)
		}
	}
	for class := range excludes {
		if !known[class] {
			return RecoverySelection{}, fmt.Errorf("recovery state class %q is not present for this application", class)
		}
	}
	return out, nil
}

func (s RecoverySelection) ValidateForCapture() error {
	for _, c := range s.Contributors {
		if c.StateClass == StateApplicationMetadata && !c.Selected {
			return errors.New("application.metadata is required in every recovery unit")
		}
		if c.Durable && c.Support != RecoverySupported && !c.ExplicitlyExcluded {
			return fmt.Errorf("durable recovery state %s %q is %s: %s; explicitly exclude this state class to create a partial recovery unit", c.StateClass, c.LogicalResource, c.Support, c.Reason)
		}
	}
	return nil
}

func (s RecoverySelection) HasSelected(class RecoveryStateClass) bool {
	for _, c := range s.Contributors {
		if c.StateClass == class && c.Selected {
			return true
		}
	}
	return false
}

func (s RecoverySelection) Selected() []RecoveryContributor {
	var selected []RecoveryContributor
	for _, c := range s.Contributors {
		if c.Selected {
			selected = append(selected, c)
		}
	}
	return selected
}

func ParseRecoveryStateClass(value string) (RecoveryStateClass, error) {
	class := RecoveryStateClass(strings.TrimSpace(value))
	switch class {
	case StateApplicationMetadata, StateSecrets, StateSQL, StateObjectStorage, StateWorkloadStorage, StateLogs, StateMetrics, StateTraces, StatePKI, StateIdentity:
		return class, nil
	default:
		return "", fmt.Errorf("unknown recovery state class %q", value)
	}
}

func recoveryClassSet(classes []RecoveryStateClass) (map[RecoveryStateClass]struct{}, error) {
	out := make(map[RecoveryStateClass]struct{}, len(classes))
	for _, class := range classes {
		if _, err := ParseRecoveryStateClass(string(class)); err != nil {
			return nil, err
		}
		out[class] = struct{}{}
	}
	return out, nil
}
