package logs

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// applicationLogProtocol is the runtime-neutral wire the workload's own
// stdout/stderr takes into the collector, independent of the transport the
// selected Runtime Provider uses: the Docker realization ships it over the
// syslog-rfc5424 listener, the Podman realization through the journald source,
// and both relabel it into the same semantic labels documented in
// docs/architecture/runtime-log-source-contract.md. Class application
// therefore records the semantic identity, never a port or a journal path -
// which is also why its provider attribution is Loki, the collector every
// transport path pushes into (the query.go selector census already attributes
// provider streams to Loki this way).
const applicationLogProtocol = "stdout-stderr"

// applicationLogServiceName is the service token of the workload identity
// class-application log signals carry in their registry ID. The concrete
// workload service is part of the collector's stream labels, not of the
// reconciliation identity: one deployment owns exactly one workload log
// signal set, named by the project/service target it registers.
const applicationLogServiceName = "workload"

// validateApplicationLogSource is the single identity contract every class
// application log registration shares: the caller describes one concrete
// workload service of this deployment's application/environment, and foreign
// identities fail closed instead of silently registering another tenant's
// stream.
func validateApplicationLogSource(m application.Manifest, source LogSource) error {
	if strings.TrimSpace(source.Application) == "" || strings.TrimSpace(source.Environment) == "" ||
		strings.TrimSpace(source.Service) == "" || source.Class == "" {
		return errors.New("log source identity is incomplete")
	}
	if source.Application != m.Name || source.Environment != m.Environment {
		return errors.New("log source does not belong to realization application")
	}
	if source.Class != application.LogsSourceApplication {
		return fmt.Errorf("runtime Loki application registration does not support source class %q", source.Class)
	}
	return nil
}

// applicationLogSource builds the class-application log signal for one
// workload service of a deployment and validates it against the registry
// contract.
func applicationLogSource(m application.Manifest, project, service string) (observability.SignalSource, error) {
	service = strings.TrimSpace(service)
	if service == "" {
		service = applicationLogServiceName
	}
	if strings.TrimSpace(project) == "" {
		return observability.SignalSource{}, fmt.Errorf("application log source %q requires its runtime project", service)
	}
	// The ID names the workload identity ("workload"), never the concrete
	// service, so the registry holds exactly one class-application log signal
	// per application/environment. The Target carries the caller's concrete
	// service, which is what the collector labels the stream with.
	source := observability.SignalSource{
		ID:                 observability.ApplicationLogSourceID(m.Name, m.Environment, applicationLogServiceName),
		Kind:               observability.SignalLogs,
		Provider:           capability.ProviderLoki,
		Class:              observability.SourceApplication,
		Scope:              capability.ScopeApplication,
		OwnerApplication:   m.Name,
		Target:             observability.RuntimeTarget(project, service),
		Protocol:           applicationLogProtocol,
		Mode:               capability.ObservabilityRuntime,
		SemanticConvention: "baseharbor.runtime.logs",
		Verification:       capability.ObservabilityVerifyBackend,
	}
	if err := source.Validate(); err != nil {
		return observability.SignalSource{}, fmt.Errorf("application log source %q: %w", service, err)
	}
	return source, nil
}

// applicationLogBoundary is the local mirror of observability.ApplicationLogBoundary
// for this package's writers; it exists so ReconcileWorkloadLogSource can be
// called with its own (m, project) arguments without importing a second
// predicate helper.
func applicationLogBoundary(m application.Manifest) func(observability.SignalSource) bool {
	return observability.ApplicationLogBoundary(m.Name, m.Environment)
}

// ReconcileApplicationLogSource writes - or reclaims - the class-application
// log signals of one deployment's workload project in the shared observability
// registry. Before card 0428-122 no production path ever stamped class
// application on a log signal: the collector already labelled the streams it
// ingests (render.go), but the registry - the listing every policy, evidence
// and Console consumer reads - stayed empty for the class, so "application
// logs collected" was reported from a source no reader could see.
//
// The writer is transport-neutral: both the render-time reconciliation in
// internal/application (Docker syslog and Podman journald alike) and the Loki
// driver Bind path land here. The registry identity is the workload token:
// exactly one class-application log signal per application/environment, whose
// Target names the resolved stream (the caller's first service, else the
// canonical workload token). A disabled deployment policy reclaims every
// signal of the application/environment boundary: the registry never keeps a
// source the deployment stopped collecting.
func ReconcileApplicationLogSource(m application.Manifest, project string, services []string) error {
	policy, err := application.LogsPolicy(m)
	if err != nil {
		return err
	}
	desired := []observability.SignalSource{}
	if policy.Enabled && policy.Collect[application.LogsSourceApplication] {
		// An enabled policy always writes exactly one signal: the ID names
		// the workload identity, the Target the resolved stream (first named
		// service, else the canonical workload token). Repeated reconciles
		// replace, never accumulate.
		source, err := applicationLogSource(m, project, firstNamedService(services))
		if err != nil {
			return err
		}
		desired = append(desired, source)
	}
	return observability.ReconcileMatching(applicationLogBoundary(m), desired)
}

func firstNamedService(services []string) string {
	for _, candidate := range services {
		if candidate = strings.TrimSpace(candidate); candidate != "" {
			return candidate
		}
	}
	return applicationLogServiceName
}

// ListWorkloadLogSources is the policy-aware listing boundary for class
// application log sources: it calls the existing generic List directly with
// its own manifest's application-collect bit, so no listing function in the
// registry changes semantics (the card asked for a writer, not a new filter).
// ListLogs keeps its provider-only defaults for every existing consumer.
func ListWorkloadLogSources(placement capability.ProviderPlacement, m application.Manifest) ([]observability.SignalSource, error) {
	policy, err := application.LogsPolicy(m)
	if err != nil {
		return nil, err
	}
	return observability.List(observability.SignalLogs, placement, []string{m.Name},
		policy.Enabled && policy.Collect[application.LogsSourceApplication], false, false)
}

// ConvergeManagedLogsForReconcile is the collector-side entry of the
// application-class log writer. The render-time writer lives in
// internal/application, where the project identity is canonical; this entry
// serves the callers that own a registration of their own - the Loki driver
// Bind path lands in ReconcileWorkloadLogSource below.
func ConvergeManagedLogsForReconcile(m application.Manifest, project string) error {
	return ReconcileApplicationLogSource(m, project, []string{applicationLogServiceName})
}

// ReconcileWorkloadLogSource registers one workload service's class-
// application log signal - the same identity ReconcileApplicationLogSource
// writes, reached directly by the Loki driver Bind path, which binds one
// concrete service at a time. An enabled policy writes exactly one signal per
// application/environment; a disabled policy fails closed here because reclaim
// is the reconciliation entry's job, not the bind path's.
func ReconcileWorkloadLogSource(m application.Manifest, project, service string) error {
	policy, err := application.LogsPolicy(m)
	if err != nil {
		return err
	}
	if !policy.Enabled || !policy.Collect[application.LogsSourceApplication] {
		return errors.New("application log collection is disabled by deployment policy")
	}
	source, err := applicationLogSource(m, project, service)
	if err != nil {
		return err
	}
	return observability.ReconcileMatching(applicationLogBoundary(m), []observability.SignalSource{source})
}
