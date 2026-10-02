package availability

import (
	"errors"
	"fmt"
	"strings"
)

type SupportLevel string

const (
	Supported          SupportLevel = "SUPPORTED"
	PartiallySupported SupportLevel = "PARTIALLY_SUPPORTED"
	Unsupported        SupportLevel = "UNSUPPORTED"
)

type Support struct {
	Level                SupportLevel `json:"level"`
	RecommendedInstances int          `json:"recommended_instances,omitempty"`
	Limits               string       `json:"limits,omitempty"`
}

func (s Support) Validate() error {
	switch s.Level {
	case Supported, PartiallySupported, Unsupported:
	default:
		return fmt.Errorf("invalid availability support level %q", s.Level)
	}
	if s.RecommendedInstances < 0 {
		return errors.New("recommended availability instance count must not be negative")
	}
	if s.Level == Unsupported && s.RecommendedInstances != 0 {
		return errors.New("unsupported availability cannot recommend instances")
	}
	return nil
}

type NegotiationResult struct {
	Component          string       `json:"component"`
	Provider           string       `json:"provider"`
	RequiredHA         bool         `json:"required_ha"`
	RequestedInstances int          `json:"requested_instances,omitempty"`
	EffectiveInstances int          `json:"effective_instances,omitempty"`
	Support            SupportLevel `json:"support"`
	ExplicitException  bool         `json:"explicit_exception,omitempty"`
	Satisfied          bool         `json:"satisfied"`
	Reason             string       `json:"reason,omitempty"`
}

type UnsupportedGuaranteeError struct {
	Result NegotiationResult
}

func (e *UnsupportedGuaranteeError) Error() string {
	return fmt.Sprintf("availability guarantee unsupported for %s by %s: %s", e.Result.Component, e.Result.Provider, e.Result.Reason)
}

func Negotiate(requirement Requirement, provider string, support Support) (NegotiationResult, error) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return NegotiationResult{}, errors.New("availability provider is required")
	}
	if err := support.Validate(); err != nil {
		return NegotiationResult{}, err
	}
	result := NegotiationResult{
		Component: requirement.Component, Provider: provider,
		RequiredHA: requirement.HA, RequestedInstances: requirement.Instances,
		ExplicitException: requirement.ExplicitException,
		Support: support.Level,
	}
	if !requirement.HA {
		result.Satisfied = true
		if requirement.Instances > 0 {
			result.EffectiveInstances = requirement.Instances
		}
		return result, nil
	}
	if support.Level == Unsupported {
		result.Reason = firstNonEmpty(support.Limits, "provider/runtime does not implement the requested HA guarantee")
		return result, &UnsupportedGuaranteeError{Result: result}
	}
	if support.Level == PartiallySupported && support.Limits != "" {
		result.Reason = support.Limits
	}
	switch {
	case requirement.Instances > 0:
		result.EffectiveInstances = requirement.Instances
	case support.RecommendedInstances > 0:
		result.EffectiveInstances = support.RecommendedInstances
	default:
		result.EffectiveInstances = 3
	}
	if result.EffectiveInstances < 2 {
		result.Reason = "effective HA topology has fewer than two instances"
		return result, &UnsupportedGuaranteeError{Result: result}
	}
	result.Satisfied = true
	return result, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
