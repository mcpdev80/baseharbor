package application

import (
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// RetainPendingProviderOwnersAt records intended realization owners in the
// existing registry before apply can partially materialize them. It does not
// publish desired bindings or release previous consumers. Native inventory must
// still verify whether an intended realization actually exists.
//
// While the retained owner is still unbound (no consumer reference was
// published for it), the registry entry alone is a projection of an intention,
// not a realization that ever existed. Destroy reconciliation treats such an
// owner as never-realized and reclaims it. The first publish that binds the
// owner re-enters the normal protected path, so a realized provider never
// loses its canonical owner through this seam.
func RetainPendingProviderOwnersAt(dataDir string, m Manifest, additional ...capability.Resource) error {
	store, err := referenceProviderRegistryStoreAt(dataDir)
	if err != nil {
		return err
	}
	return store.Update(func(registry *capability.Registry) error {
		// Build a separate, validated projection so a placement replacement can
		// be checked without overwriting its currently applied binding.
		projection := capability.NewRegistry()
		projection.Instances = append(projection.Instances, registry.Instances...)
		if err := registerReferenceProvidersAt(dataDir, &projection, m); err != nil {
			return err
		}
		if err := registerAdditionalProviderResources(&projection, m, additional); err != nil {
			return err
		}
		if err := projection.Validate(); err != nil {
			return err
		}
		for _, instance := range projection.Instances {
			if err := registry.Register(instance); err != nil {
				return err
			}
			if instance.Scope != capability.ScopeApplication || instance.OwnerApplicationID != m.ApplicationID {
				continue
			}
			if len(registry.BindingsForProviderInstance(instance.ID)) != 0 {
				continue
			}
			// The pending owner is unbound in the currently applied registry.
			// Reconcile it against the canonical projection so an interrupted
			// first apply cannot strand an unbindable (or placement-changed)
			// provider instance that later destroy must refuse to verify.
			canonical, err := referenceProviderInstance(m, capability.Resource{
				Application: m.Name,
				Kind:        instance.Provider.Capabilities[0],
				Name:        "default",
				Provider:    instance.Provider.Kind,
			})
			if err != nil {
				return fmt.Errorf("reconcile pending provider owner %q: %w", instance.ID, err)
			}
			if canonical.ID != instance.ID {
				return fmt.Errorf("pending provider owner %q differs from its canonical realization %q", instance.ID, canonical.ID)
			}
			if err := registry.Register(canonical); err != nil {
				return err
			}
		}
		return nil
	})
}
