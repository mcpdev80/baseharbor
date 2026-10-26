package builtin

import (
	"github.com/mcpdev80/baseharbor/internal/extension"
)

// ExtensionMetadata projects the installable identity of a bundled capability
// provider into the common extension-distribution metadata model. Provider
// lifecycle semantics remain owned by capability.IntegrationDescriptor.
func ExtensionMetadata(descriptor Descriptor) extension.Metadata {
	contracts := make([]string, 0, len(descriptor.Integration.Capabilities))
	for _, contract := range descriptor.Integration.Capabilities {
		contracts = append(contracts, string(contract))
	}
	return extension.Metadata{
		SchemaVersion: extension.DescriptorVersion,
		ID:            descriptor.ID,
		Family:        extension.FamilyProvider,
		Version:       descriptor.Version,
		Compatibility: extension.Compatibility{
			Contracts: contracts,
		},
	}
}
