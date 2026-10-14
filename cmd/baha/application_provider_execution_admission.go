package main

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
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
	if err != nil {
		return err
	}
	if application.HasIdentity(resolved.Manifest) && !isRemoteApplication(resolved) {
		selected, err := application.IdentityProviderForDeployment()
		if err != nil {
			return err
		}
		if selected.Kind == capability.ProviderExternalOIDC {
			if _, err := identityprovider.NewRegisteredExternalDriver(resolved.Manifest, application.RuntimeFilesFor(resolved.Store, resolved.Manifest), resolved.TargetStateRoot); err != nil {
				return &machine.Error{Code: machine.ErrorInvalidWorkload, CauseCode: "external_provider_material_preflight_failed", Message: "Selected external OIDC registration or material cannot be safely delivered.", Next: "Inspect the canonical registration's issuer, client and protected credential/trust references before execution; current delivery remains unchanged.", Details: impact}
			}
		}
	}
	if application.HasOTLPTelemetry(resolved.Manifest) && application.TelemetryProviderForDeployment().Kind == capability.ProviderExternalOTLP && !isRemoteApplication(resolved) {
		driver := telemetry.NewDriver(nil, resolved.Manifest, application.RuntimeFilesFor(resolved.Store, resolved.Manifest), nil)
		resource := capability.Resource{Kind: capability.TelemetryOTLP, Provider: capability.ProviderExternalOTLP}
		binding := capability.Binding{TelemetryOTLP: &capability.OTLPTelemetryBinding{Direction: "export", Protocol: "http/protobuf", Signals: resolved.Manifest.Telemetry.OTLP.Signals}}
		if err := driver.Preflight(context.Background(), resource, binding); err != nil {
			return &machine.Error{Code: machine.ErrorInvalidWorkload, CauseCode: "external_provider_material_preflight_failed", Message: "Selected external OTLP material or endpoint cannot be safely delivered.", Next: "Recover the protected deployment owner or explicitly select the destination's authorization and valid HTTPS receiver; current delivery remains unchanged.", Details: impact}
		}
	}
	return nil
}
