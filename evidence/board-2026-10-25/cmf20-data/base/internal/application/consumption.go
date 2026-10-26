package application

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
)

// ConsumptionRequirement is portable application intent for one logical
// application/component interface consumed by this application.
//
// It deliberately contains no runtime address, namespace, container, Pod,
// node, replica or provider product identity.
type ConsumptionRequirement struct {
	Name          string `json:"name"`
	ApplicationID string `json:"application_id,omitempty"`
	Component     string `json:"component"`
	Interface     string `json:"interface"`
	Protocol      string `json:"protocol,omitempty"`
}

type LogicalProducerReference struct {
	ApplicationID string `json:"application_id"`
	Component     string `json:"component"`
	Interface     string `json:"interface"`
}

type ResolvedConsumption struct {
	Name       string                   `json:"name"`
	Producer   LogicalProducerReference `json:"producer"`
	BindingID  string                   `json:"binding_id"`
	Protocol   string                   `json:"protocol,omitempty"`
	Endpoint   string                   `json:"endpoint,omitempty"`
	Resolved   bool                     `json:"resolved"`
	Diagnostic string                   `json:"diagnostic,omitempty"`
}

func (r ConsumptionRequirement) Producer(currentApplicationID string) (LogicalProducerReference, error) {
	appID := strings.TrimSpace(r.ApplicationID)
	if appID == "" {
		appID = strings.TrimSpace(currentApplicationID)
	}
	if err := ValidateApplicationID(appID); err != nil {
		return LogicalProducerReference{}, fmt.Errorf("consumption %q producer application: %w", r.Name, err)
	}
	if err := validateWorkloadComponentName(strings.TrimSpace(r.Component)); err != nil {
		return LogicalProducerReference{}, fmt.Errorf("consumption %q producer component: %w", r.Name, err)
	}
	if strings.TrimSpace(r.Interface) == "" {
		return LogicalProducerReference{}, fmt.Errorf("consumption %q producer interface is required", r.Name)
	}
	return LogicalProducerReference{ApplicationID: appID, Component: strings.TrimSpace(r.Component), Interface: strings.TrimSpace(r.Interface)}, nil
}

func (r ConsumptionRequirement) Validate(currentApplicationID string) error {
	if err := validateSlug("consumption name", strings.TrimSpace(r.Name)); err != nil {
		return err
	}
	if _, err := r.Producer(currentApplicationID); err != nil {
		return err
	}
	protocol := strings.TrimSpace(r.Protocol)
	if strings.ContainsAny(protocol, " /\\:@") {
		return fmt.Errorf("consumption %q protocol %q must be a logical protocol identifier, not an address", r.Name, protocol)
	}
	return nil
}

func ConsumptionBindingID(producer LogicalProducerReference) string {
	sum := sha256.Sum256([]byte(producer.ApplicationID + "\x00" + producer.Component + "\x00" + producer.Interface))
	return fmt.Sprintf("consumption:%x", sum[:12])
}

// ResolveConsumption applies runtime/provider realization without changing the
// logical producer reference. Endpoint is realization state, never portable
// application intent.
func ResolveConsumption(currentApplicationID string, requirement ConsumptionRequirement, endpoint string) (ResolvedConsumption, error) {
	producer, err := requirement.Producer(currentApplicationID)
	if err != nil {
		return ResolvedConsumption{}, err
	}
	result := ResolvedConsumption{
		Name: requirement.Name, Producer: producer, BindingID: ConsumptionBindingID(producer), Protocol: strings.TrimSpace(requirement.Protocol),
	}
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		result.Diagnostic = "logical producer has no resolved stable endpoint"
		return result, nil
	}
	if strings.ContainsAny(endpoint, "\r\n\x00") {
		return ResolvedConsumption{}, errors.New("resolved consumption endpoint is invalid")
	}
	result.Endpoint = endpoint
	result.Resolved = true
	return result, nil
}
