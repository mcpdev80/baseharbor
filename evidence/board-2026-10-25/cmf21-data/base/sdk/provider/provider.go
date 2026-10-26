// Package provider exposes the stable Capability Provider authoring boundary.
//
// Product-specific provider implementations depend on this package rather than
// BaseHarbor internal packages. Portable application intent remains independent
// of provider products.
package provider

import (
	"context"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/providerconformance"
	"github.com/mcpdev80/baseharbor/internal/reconciliation"
)

const ProtocolV1 = capability.ProviderProtocolV1

type (
	Kind                   = capability.Kind
	Requirement            = capability.Requirement
	ProviderKind           = capability.ProviderKind
	Provider               = capability.Provider
	Resource               = capability.Resource
	Binding                = capability.Binding
	HTTPExposureBinding    = capability.HTTPExposureBinding
	ObjectStorageS3Binding = capability.ObjectStorageS3Binding
	OTLPTelemetryBinding   = capability.OTLPTelemetryBinding
	MetricsBinding         = capability.MetricsBinding
	LogsBinding            = capability.LogsBinding
	IdentityBinding        = capability.IdentityBinding
	SecureBinding          = capability.SecureBinding
	Driver                 = capability.Driver
	ReconciliationDriver   = capability.ReconciliationDriver
	Request                = capability.Request
	Plan                   = capability.Plan
	Result                 = capability.Result
	Execution              = capability.Execution

	IntegrationDescriptor       = capability.IntegrationDescriptor
	OptionalLifecycleSupport    = capability.OptionalLifecycleSupport
	ProviderScope               = capability.ProviderScope
	ProviderOwnership           = capability.ProviderOwnership
	ProviderPlacement           = capability.ProviderPlacement
	ServiceKind                 = capability.ServiceKind
	SpecificationID             = capability.SpecificationID
	ProviderInterface           = capability.ProviderInterface
	ProviderInterfaceClass      = capability.ProviderInterfaceClass
	ProviderObservability       = capability.ProviderObservability
	ProviderObservabilitySignal = capability.ProviderObservabilitySignal

	Desired                 = reconciliation.Desired
	Observed                = reconciliation.Observed
	ReconciliationResult    = reconciliation.Result
	ReconciliationState     = reconciliation.State
	ReconciliationAction    = reconciliation.Action
	ReconciliationOwnership = reconciliation.Ownership

	ConformanceStatus = providerconformance.Status
	ConformanceCheck  = providerconformance.Check
	ConformanceReport = providerconformance.Report
)

const (
	SQL              = capability.SQL
	KeyValue         = capability.KeyValue
	DurableKeyValue  = capability.DurableKeyValue
	DocumentDatabase = capability.DocumentDatabase
	Secrets          = capability.Secrets
	ExposureHTTP     = capability.ExposureHTTP
	ObjectStorageS3  = capability.ObjectStorageS3
	TelemetryOTLP    = capability.TelemetryOTLP
	Metrics          = capability.Metrics
	Logs             = capability.Logs
	Traces           = capability.Traces
	Identity         = capability.Identity
	MessagingQueue   = capability.MessagingQueue
	MessagingPubSub  = capability.MessagingPubSub
	MessagingStream  = capability.MessagingStream

	ScopeShared      = capability.ScopeShared
	ScopeApplication = capability.ScopeApplication
	ScopeExternal    = capability.ScopeExternal

	OwnershipBaseHarbor = capability.OwnershipBaseHarbor
	OwnershipExternal   = capability.OwnershipExternal

	ConformancePass = providerconformance.Pass
	ConformanceFail = providerconformance.Fail
)

type ConformanceTarget struct {
	Descriptor       IntegrationDescriptor
	Request          Request
	Application      string
	UnsupportedScope ProviderScope
}

func BuildPlan(application string, requests []Request) (Plan, error) {
	return capability.BuildPlan(application, requests)
}

func Prepare(ctx context.Context, application string, requests []Request) (*Execution, Result, error) {
	return capability.Prepare(ctx, application, requests)
}

func CheckIntegrationContract(descriptor IntegrationDescriptor) capability.ConformanceReport {
	return capability.CheckIntegrationContract(descriptor)
}

func RunConformance(ctx context.Context, target ConformanceTarget) ConformanceReport {
	return providerconformance.Run(ctx, providerconformance.Target{
		Descriptor:       target.Descriptor,
		Request:          target.Request,
		Application:      target.Application,
		UnsupportedScope: target.UnsupportedScope,
	})
}

func RequireConformance(report ConformanceReport) error {
	return providerconformance.Require(report)
}
