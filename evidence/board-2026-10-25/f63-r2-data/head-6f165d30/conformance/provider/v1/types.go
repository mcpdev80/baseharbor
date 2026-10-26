package provider

import (
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/providerconformance"
	"github.com/mcpdev80/baseharbor/internal/reconciliation"
)

// Public aliases let external implementers satisfy the same Core interfaces
// without importing internal packages or maintaining a parallel protocol.
type Driver = capability.Driver
type Provider = capability.Provider
type ProviderKind = capability.ProviderKind
type Kind = capability.Kind
type Requirement = capability.Requirement
type Resource = capability.Resource
type Binding = capability.Binding
type Request = capability.Request
type IntegrationDescriptor = capability.IntegrationDescriptor
type SpecificationID = capability.SpecificationID
type ServiceKind = capability.ServiceKind
type ProviderScope = capability.ProviderScope
type ProviderInterface = capability.ProviderInterface
type OptionalLifecycleSupport = capability.OptionalLifecycleSupport
type ProviderObservability = capability.ProviderObservability
type ProviderObservabilitySignal = capability.ProviderObservabilitySignal
type ProviderOperationObserver = capability.ProviderOperationObserver
type ProviderOperationObservation = capability.ProviderOperationObservation
type SecureBinding = capability.SecureBinding
type HTTPExposureBinding = capability.HTTPExposureBinding
type ObjectStorageS3Binding = capability.ObjectStorageS3Binding
type OTLPTelemetryBinding = capability.OTLPTelemetryBinding
type MetricsBinding = capability.MetricsBinding
type LogsBinding = capability.LogsBinding
type IdentityBinding = capability.IdentityBinding
type ReconciliationDriver = capability.ReconciliationDriver
type Desired = reconciliation.Desired
type Observed = reconciliation.Observed
type StateDigester = providerconformance.StateDigester
type Drifter = providerconformance.Drifter
type Destroyer = providerconformance.Destroyer
type FaultFixture = providerconformance.FaultFixture
type OwnershipFixture = providerconformance.OwnershipFixture
type FailureMode = providerconformance.FailureMode
type Ownership = reconciliation.Ownership

const (
	FailureNone             = providerconformance.FailureNone
	FailureUnavailable      = providerconformance.FailureUnavailable
	FailureProvision        = providerconformance.FailureProvision
	FailureMalformedBinding = providerconformance.FailureMalformedBinding
	FailureVerify           = providerconformance.FailureVerify
	OwnershipBaseHarbor     = reconciliation.OwnershipBaseHarbor
	OwnershipForeign        = reconciliation.OwnershipForeign
	ProtocolVersion         = capability.ProviderProtocolV1
	ScopeApplication        = capability.ScopeApplication
	ScopeShared             = capability.ScopeShared
	ScopeExternal           = capability.ScopeExternal
)
