package application

import "github.com/mcpdev80/baseharbor/internal/capability"

// CapabilityKind and CapabilityRequirement remain aliases in the application
// package so existing v0.4 callers keep their source-compatible contract while
// the reusable capability domain moves into the shared core.
type CapabilityKind = capability.Kind

const (
	CapabilitySQL          CapabilityKind = capability.SQL
	CapabilityKeyValue     CapabilityKind = capability.KeyValue
	CapabilityExposureHTTP CapabilityKind = capability.ExposureHTTP\n\tCapabilityObjectStorageS3 CapabilityKind = capability.ObjectStorageS3
)

type CapabilityRequirement = capability.Requirement

// SecretContract describes secret intent without selecting a concrete secret
// backend or delivery mechanism. Required values remain names/generation intent
// only; provider credentials and provider object identifiers never belong here.
type SecretContract struct {
	Managed  bool
	Required []SecretRequirement
}

// PortableContract is the provider-neutral application requirement view used
// as the seam between manifest compatibility and runtime/capability providers.
//
// Deployment context such as environment, Compose paths, host ports, TLS source
// directories and provider object names must not be added here. Those belong to
// deployment state or provider-specific configuration.
type PortableContract struct {
	Application  string
	Capabilities []CapabilityRequirement
	Secrets      SecretContract
	Exposures    []HTTPExposureRequirement
}

// PortableContractFromManifest translates the current manifest v1 compatibility
// surface into provider-neutral requirements without changing manifest behavior.
// The translation is deliberately one-way for now: v0.4 can evolve a future
// contract schema while existing v0.3 manifests continue to load unchanged.
func PortableContractFromManifest(m Manifest) (PortableContract, error) {
	if err := m.Validate(); err != nil {
		return PortableContract{}, err
	}

	contract := PortableContract{
		Application: m.Name,
		Secrets: SecretContract{
			Managed:  m.Services.Secrets,
			Required: cloneSecretRequirements(m.Secrets.Required),
		},
		Exposures: append([]HTTPExposureRequirement(nil), m.Exposures...),
	}
	for _, name := range PostgresInstanceNames(m) {
		contract.Capabilities = append(contract.Capabilities, CapabilityRequirement{
			Kind: CapabilitySQL,
			Name: name,
		})
	}
	for _, name := range RedisInstanceNames(m) {
		contract.Capabilities = append(contract.Capabilities, CapabilityRequirement{
			Kind: CapabilityKeyValue,
			Name: name,
		})
	}
	for _, exposure := range m.Exposures {
		contract.Capabilities = append(contract.Capabilities, CapabilityRequirement{
			Kind: CapabilityExposureHTTP,
			Name: exposure.Name,
		})
	}

	return contract, nil
}

func cloneSecretRequirements(requirements []SecretRequirement) []SecretRequirement {
	if len(requirements) == 0 {
		return nil
	}
	cloned := make([]SecretRequirement, 0, len(requirements))
	for _, requirement := range requirements {
		copyRequirement := requirement
		if requirement.Generate != nil {
			generation := *requirement.Generate
			copyRequirement.Generate = &generation
		}
		cloned = append(cloned, copyRequirement)
	}
	return cloned
}
