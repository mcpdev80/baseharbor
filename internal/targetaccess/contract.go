package targetaccess

import (
	"fmt"
	"regexp"
	"strings"
)

const ContractVersion = "baseharbor.target-access/v1"

type ProviderKind string

const (
	ProviderLocal         ProviderKind = "local"
	ProviderNodeConnector ProviderKind = "baseharbor-node-connector"
	ProviderNativeAPI     ProviderKind = "native-api"
)

type Capability string

const (
	CapabilityConnect          Capability = "connect"
	CapabilityStream           Capability = "stream"
	CapabilityExecTransport    Capability = "exec-transport"
	CapabilityTypedRealization Capability = "typed-realization"
	CapabilityPeerIdentity     Capability = "peer-identity"
	CapabilityNativeContext    Capability = "native-context"
)

type Capabilities struct {
	Connect          bool `json:"connect"`
	Stream           bool `json:"stream"`
	ExecTransport    bool `json:"exec_transport"`
	TypedRealization bool `json:"typed_realization"`
	PeerIdentity     bool `json:"peer_identity"`
	NativeContext    bool `json:"native_context"`
}

func (c Capabilities) Supports(capability Capability) bool {
	switch capability {
	case CapabilityConnect:
		return c.Connect
	case CapabilityStream:
		return c.Stream
	case CapabilityExecTransport:
		return c.ExecTransport
	case CapabilityTypedRealization:
		return c.TypedRealization
	case CapabilityPeerIdentity:
		return c.PeerIdentity
	case CapabilityNativeContext:
		return c.NativeContext
	default:
		return false
	}
}

type Descriptor struct {
	Kind            ProviderKind `json:"kind"`
	ContractVersion string       `json:"contract_version"`
	Remote          bool         `json:"remote"`
	Capabilities    Capabilities `json:"capabilities"`
}

var providerKindPattern = regexp.MustCompile("^[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?(?:/[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?)*$")

func ParseProviderKind(value string) (ProviderKind, error) {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "" {
		return "", fmt.Errorf("target access provider is required")
	}
	if !providerKindPattern.MatchString(normalized) {
		return "", fmt.Errorf("invalid target access provider %q", value)
	}
	return ProviderKind(normalized), nil
}

func BuiltInDescriptor(kind ProviderKind) (Descriptor, bool) {
	switch kind {
	case ProviderLocal:
		return Descriptor{
			Kind:            ProviderLocal,
			ContractVersion: ContractVersion,
			Remote:          false,
			Capabilities: Capabilities{
				Connect: true, Stream: true, ExecTransport: true, TypedRealization: true,
			},
		}, true
	case ProviderNodeConnector:
		return Descriptor{
			Kind:            ProviderNodeConnector,
			ContractVersion: ContractVersion,
			Remote:          true,
			Capabilities: Capabilities{
				Connect: true, Stream: true, ExecTransport: true, TypedRealization: true, PeerIdentity: true,
			},
		}, true
	case ProviderNativeAPI:
		return Descriptor{
			Kind:            ProviderNativeAPI,
			ContractVersion: ContractVersion,
			Remote:          true,
			Capabilities: Capabilities{
				Connect: true, Stream: true, ExecTransport: true, TypedRealization: true, PeerIdentity: true, NativeContext: true,
			},
		}, true
	default:
		return Descriptor{}, false
	}
}

func RequireCapabilities(descriptor Descriptor, required ...Capability) error {
	if descriptor.ContractVersion != ContractVersion {
		return fmt.Errorf("target access provider %q uses contract %q, require %q", descriptor.Kind, descriptor.ContractVersion, ContractVersion)
	}
	for _, capability := range required {
		if capability == "" {
			return fmt.Errorf("target access provider %q: empty capability requirement", descriptor.Kind)
		}
		if !descriptor.Capabilities.Supports(capability) {
			return fmt.Errorf("target access provider %q does not support required capability %q", descriptor.Kind, capability)
		}
	}
	return nil
}
