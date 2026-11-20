package metrics

import (
	"encoding/json"
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

const (
	PersesMessagingDashboardKey         = "rabbitmq-messaging"
	PersesMessagingUnavailableKind      = "DashboardUnavailable"
	PersesMessagingDashboardAvailable   = "available"
	PersesMessagingDashboardUnavailable = "unavailable"
)

// PersesMessagingDashboardUnavailableDocument explains why BaseHarbor does
// not provision a RabbitMQ dashboard. It is a catalog result, not a Perses
// Dashboard resource, so it cannot introduce empty or fabricated panels.
type PersesMessagingDashboardUnavailableDocument struct {
	Kind        string `json:"kind"`
	Key         string `json:"key"`
	Backend     string `json:"backend"`
	Unavailable bool   `json:"unavailable"`
	Reason      string `json:"reason"`
}

// PersesMessagingDashboardEntry is the RabbitMQ catalog result. Messaging
// currently has no native signal, so no Perses dashboard is emitted and the
// entry is explicitly unavailable.
type PersesMessagingDashboardEntry struct {
	Key         string
	Backend     string
	Unavailable bool
	Reason      string
}

// PersesMessagingCatalog carries the accepted Perses release used for
// eligibility checks.
type PersesMessagingCatalog struct {
	release capability.PersesRelease
}

// DefaultPersesMessagingCatalog returns the conditional RabbitMQ
// entry. Availability is grounded in the integration descriptor; a supplied
// signal inventory cannot override the absence of a declared native signal.
func DefaultPersesMessagingCatalog() PersesMessagingCatalog {
	return PersesMessagingCatalog{release: capability.PersesDefaultRelease}
}

// DashboardCatalog returns the one fail-closed messaging entry.
func (c PersesMessagingCatalog) DashboardCatalog() map[string]PersesMessagingDashboardEntry {
	reason := "RabbitMQ has no native observability signal in BaseHarbor; the dashboard is unavailable and no exporter is installed"
	if err := capability.ValidatePersesRelease(c.release); err != nil {
		reason = "Perses release is not pinned or is invalid (fail-closed); RabbitMQ dashboard remains unavailable"
	}
	if len(capability.RabbitMQIntegration.Observability.Signals) != 0 {
		reason = "RabbitMQ has no validated native Perses signal in BaseHarbor; the dashboard is unavailable and no exporter is installed"
	}
	return map[string]PersesMessagingDashboardEntry{
		PersesMessagingDashboardKey: {
			Key:         PersesMessagingDashboardKey,
			Backend:     string(capability.ProviderRabbitMQ),
			Unavailable: true,
			Reason:      reason,
		},
	}
}

// PersesMessagingDashboardAvailability reports the single dashboard's explicit
// availability result without changing provider selection or installing an
// exporter.
func PersesMessagingDashboardAvailability() map[string]PersesMessagingDashboardAvailabilityEntry {
	entry := DefaultPersesMessagingCatalog().DashboardCatalog()[PersesMessagingDashboardKey]
	return map[string]PersesMessagingDashboardAvailabilityEntry{
		entry.Key: {
			Key:       entry.Key,
			Backend:   entry.Backend,
			Available: PersesMessagingDashboardUnavailable,
			Reason:    entry.Reason,
		},
	}
}

// PersesMessagingDashboardAvailabilityEntry is the machine-readable status
// for the RabbitMQ dashboard catalog entry.
type PersesMessagingDashboardAvailabilityEntry struct {
	Key       string `json:"key"`
	Backend   string `json:"backend"`
	Available string `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// PersesMessagingDashboardCatalogJSON emits a deterministic unavailable
// document. No Perses dashboard or query is emitted until a real native signal
// is declared, verified and accepted.
func PersesMessagingDashboardCatalogJSON() ([]byte, error) {
	entry := DefaultPersesMessagingCatalog().DashboardCatalog()[PersesMessagingDashboardKey]
	doc := PersesMessagingDashboardUnavailableDocument{
		Kind:        PersesMessagingUnavailableKind,
		Key:         entry.Key,
		Backend:     entry.Backend,
		Unavailable: true,
		Reason:      entry.Reason,
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal messaging dashboard availability: %w", err)
	}
	return append(data, '\n'), nil
}

// PersesMessagingDashboardEligibility reports whether the RabbitMQ dashboard
// can be provisioned. It always fails closed today because RabbitMQIntegration
// declares no native signal; caller-supplied inventory cannot fabricate one.
func (c PersesMessagingCatalog) EligibilityFor(persesSelected bool, backendSignals map[string]bool) PersesMessagingDashboardAvailabilityEntry {
	entry := c.DashboardCatalog()[PersesMessagingDashboardKey]
	if !persesSelected {
		entry.Reason = "Perses observability surface is not selected"
	}
	if persesSelected && capability.ValidatePersesRelease(c.release) != nil {
		entry.Reason = "Perses release is not pinned or is invalid (fail-closed)"
	}
	// A signal inventory is only evidence when the provider contract declares a
	// native metric signal. A caller-supplied boolean cannot create that contract.
	declaresNativeMetrics := hasVerifiedNativeMetrics(capability.RabbitMQIntegration.Observability.Signals)
	if persesSelected && capability.ValidatePersesRelease(c.release) == nil &&
		declaresNativeMetrics && backendSignals[string(capability.ProviderRabbitMQ)] {
		entry.Reason = "RabbitMQ has a native metric signal, but no validated Perses dashboard is available"
	}
	return PersesMessagingDashboardAvailabilityEntry{
		Key:       entry.Key,
		Backend:   entry.Backend,
		Available: PersesMessagingDashboardUnavailable,
		Reason:    entry.Reason,
	}
}

func hasVerifiedNativeMetrics(signals []capability.ProviderObservabilitySignal) bool {
	for _, signal := range signals {
		if signal.Kind == capability.ObservabilityMetrics &&
			signal.Status == capability.ObservabilitySupported &&
			signal.Mode == capability.ObservabilityNative &&
			signal.Verification == capability.ObservabilityVerifyBackend {
			return true
		}
	}
	return false
}
