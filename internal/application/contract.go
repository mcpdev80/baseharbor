package application

// CapabilityKind identifies a portable application requirement. Capability
// kinds describe what an application needs, not which product or runtime
// provider satisfies it.
type CapabilityKind string

const (
	CapabilitySQL      CapabilityKind = "database.sql"
	CapabilityKeyValue CapabilityKind = "cache.key-value"
)

// CapabilityRequirement is one stable logical resource requested by an
// application. Name is application-owned identity; provider-specific topology,
// object names, ports and credentials are intentionally absent.
type CapabilityRequirement struct {
	Kind CapabilityKind
	Name string
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
	Secrets      []SecretRequirement
}

// PortableContractFromManifest translates the current manifest v1 compatibility
// surface into provider-neutral requirements without changing manifest behavior.
// The translation is deliberately one-way for now: v0.4 can evolve a future
// contract schema while existing v0.3 manifests continue to load unchanged.
func PortableContractFromManifest(m Manifest) (PortableContract, error) {
	if err := m.Validate(); err != nil {
		return PortableContract{}, err
	}

	contract := PortableContract{Application: m.Name}
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

	contract.Secrets = cloneSecretRequirements(m.Secrets.Required)
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
