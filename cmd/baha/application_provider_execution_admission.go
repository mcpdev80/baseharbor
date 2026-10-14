package main

import (
	"errors"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// This is the existing application execution boundary, not a new selector or
// provider registry. All apply/up transports reach it before pending writes.
func validateApplicationProviderExecution(resolved resolvedApplication, impact *application.ProviderImpactPlan) error {
	actual, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return err
	}
	planned, err := application.PreviewReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest)
	if err != nil {
		return err
	}
	for _, binding := range planned.Bindings {
		if binding.ApplicationID != resolved.Manifest.ApplicationID || binding.Environment != resolved.Manifest.Environment {
			continue
		}
		for _, instance := range planned.Instances {
			if instance.ID == binding.ProviderInstanceID && instance.Ownership == capability.OwnershipExternal && instance.Provider.Kind != capability.ProviderExternalOIDC && instance.Provider.Kind != capability.ProviderExternalOTLP {
				return &machine.Error{Code: machine.ErrorUnsupported, CauseCode: "external_provider_adapter_unavailable", Resource: instance.ID, Message: "The selected BYO provider has no compatible application lifecycle adapter.", Next: "Review the displayed consequence plan and provide an implemented secure adapter before execution; current bindings, runtime and data remain owned.", Details: impact}
			}
		}
	}
	err = validateApplicationBindingReplacements(resolved, actual, planned)
	var failure *machine.Error
	if errors.As(err, &failure) {
		failure.Details = impact
	}
	return err
}
