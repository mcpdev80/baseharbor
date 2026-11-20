package observability

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// ProviderSignalRegistration binds a provider integration descriptor to one
// concrete runtime instance. Multiple signal kinds intentionally share ID so
// Remove(ID) tears down all BaseHarbor-owned observability registrations for
// that provider instance.
type ProviderSignalRegistration struct {
	ID               string
	Descriptor       capability.IntegrationDescriptor
	Class            SourceClass
	Scope            capability.ProviderScope
	SharingBoundary  string
	OwnerApplication string
	Enabled          map[SignalKind]bool
	Signals          map[string]ProviderSignalRuntime
	// Legacy, when set, makes the registration migrate the pre-scoping source
	// of record INSIDE its own single mutation (card 0428-168): the legacy
	// entry is re-booked onto a derived, project-scoped identity BEFORE the
	// (ID, provider) replacement runs, so the registration can never silently
	// drop a source that predates the ID scoping.
	Legacy *LegacyReconciliation
}

func RegisterProviderSignals(registration ProviderSignalRegistration) error {
	if strings.TrimSpace(registration.ID) == "" {
		return errors.New("provider observability registration requires id")
	}
	if err := registration.Descriptor.Validate(); err != nil {
		return fmt.Errorf("provider observability descriptor: %w", err)
	}
	declared := make(map[string]capability.ProviderObservabilitySignal, len(registration.Descriptor.Observability.Signals))
	for _, signal := range registration.Descriptor.Observability.Signals {
		declared[signal.Name] = signal
	}
	for name := range registration.Signals {
		signal, ok := declared[name]
		if !ok {
			return fmt.Errorf("provider %q runtime signal %q is not declared", registration.Descriptor.Provider.Kind, name)
		}
		if !signal.Collectable() {
			return fmt.Errorf("provider %q runtime signal %q is not collectable (status %q)", registration.Descriptor.Provider.Kind, name, signal.Status)
		}
	}

	desired := make([]SignalSource, 0, len(registration.Descriptor.Observability.Signals))
	for _, signal := range registration.Descriptor.Observability.Signals {
		if !signal.Collectable() {
			continue
		}
		kind, err := signalKind(signal.Kind)
		if err != nil {
			return err
		}
		if registration.Enabled != nil && !registration.Enabled[kind] {
			continue
		}
		runtimeSignal, ok := registration.Signals[signal.Name]
		if !ok {
			return fmt.Errorf("provider %q supported signal %q has no runtime realization", registration.Descriptor.Provider.Kind, signal.Name)
		}
		source := SignalSource{
			ID:                 registration.ID,
			Kind:               kind,
			Provider:           registration.Descriptor.Provider.Kind,
			Class:              registration.Class,
			Scope:              registration.Scope,
			SharingBoundary:    registration.SharingBoundary,
			OwnerApplication:   registration.OwnerApplication,
			Network:            runtimeSignal.Network,
			Target:             runtimeSignal.Target,
			Protocol:           signal.Protocol,
			Path:               signal.Path,
			Mode:               signal.Mode,
			SemanticConvention: signal.SemanticConvention,
			Verification:       signal.Verification,
			Security:           runtimeSignal.Security,
		}
		if err := source.Validate(); err != nil {
			return fmt.Errorf("register provider %q signal %q: %w", registration.Descriptor.Provider.Kind, signal.Name, err)
		}
		desired = append(desired, source)
	}

	path, err := registryPath()
	if err != nil {
		return err
	}
	var registrationErr error
	err = mutate(path, func(sources []SignalSource) ([]SignalSource, error) {
		if legacy := registration.Legacy; legacy != nil {
			sources = RebookLegacySignals(*legacy, sources, registration.ID)
		}
		// Normalization-boundary guard (card 0428-166): the Keycloak signal ID
		// is "keycloak:" + files.Project, which runs the app name through
		// projectToken (lowercase, "." -> "-"); two DISTINCT names normalizing to
		// the same project build the SAME registrationID. The (ID, provider)
		// replacement below would otherwise let the second registration silently
		// overwrite the first owner's source. Fail closed: a registrationID
		// already held by a DIFFERENT owner is rejected (no owner vanishes); the
		// same owner is idempotent.
		for _, existing := range sources {
			if existing.ID == registration.ID && existing.Provider == registration.Descriptor.Provider.Kind &&
				existing.OwnerApplication != "" && existing.OwnerApplication != registration.OwnerApplication {
				// Keep any successful legacy rebooking from this same locked
				// mutation, while refusing to replace the scoped owner's source.
				// Returning an error from the callback would make mutateChanged
				// discard every callback edit, including that migration.
				registrationErr = fmt.Errorf("observability registration id %q already owned by %q, not %q: registration rejected (0428-166 normalization collision guard)", registration.ID, existing.OwnerApplication, registration.OwnerApplication)
				return sources, nil
			}
		}
		out := sources[:0]
		for _, existing := range sources {
			if existing.ID == registration.ID && existing.Provider == registration.Descriptor.Provider.Kind {
				continue
			}
			out = append(out, existing)
		}
		out = append(out, desired...)
		return out, nil
	})
	if err != nil {
		return err
	}
	return registrationErr
}
