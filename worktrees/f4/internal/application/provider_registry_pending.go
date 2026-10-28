package application

import "github.com/mcpdev80/baseharbor/internal/capability"

// RetainPendingProviderOwnersAt records intended realization owners in the
// existing registry before apply can partially materialize them. It does not
// publish desired bindings or release previous consumers. Native inventory must
// still verify whether an intended realization actually exists.
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
		}
		return nil
	})
}
