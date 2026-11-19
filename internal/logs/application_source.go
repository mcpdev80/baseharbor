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
// log signal of one deployment's workload project in the shared observability
// registry. Before card 0428-122 no production path ever stamped class
// application on a log signal: the collector already labelled the streams it
// ingests (render.go), but the registry - the listing every policy, evidence
// and Console consumer reads - stayed empty for the class, so "application
// logs collected" was reported from a source no reader could see.
//
// The writer is transport-neutral by design: the Loki Bind path (the Docker
// realization) and the render-time convergence entry ConvergeManagedLogsFor-
// Reconcile (both transports, wired in internal/application) all land here.
// An empty service list reclaims the deployment's workload log signal, and a
// disabled deployment policy reclaims it too: the registry never keeps a
// source the deployment stopped collecting.
func ReconcileApplicationLogSource(m application.Manifest, project string, services []string) error {
	policy, err := application.LogsPolicy(m)
	if err != nil {
		return err
	}
	desired := []observability.SignalSource{}
	if policy.Enabled && policy.Collect[application.LogsSourceApplication] {
		// One deployment collects one workload stream set. A caller that
		// resolves the concrete workload services (the render-time boundary
		// in internal/application) passes them and they win, so the registry
		// names the same identity the collector labels the stream with; a
		// caller that has only the application manifest passes an empty
		// service set and gets the canonical workload token, never a silent
		// no-op.
		service := applicationLogServiceName
		for _, candidate := range services {
			if candidate = strings.TrimSpace(candidate); candidate != "" {
				service = candidate
				break
			}
		}
		source, err := applicationLogSource(m, project, service)
		if err != nil {
			return err
		}
		desired = append(desired, source)
	}
	return observability.ReconcileMatching(applicationLogBoundary(m), desired)
}

// ListWorkloadLogSources is the policy-aware listing boundary for class
// application log sources: ListLogs keeps the provider-only defaults every
// renderer and inventory consumer depends on, so this wrapper resolves the
// caller's deployment policy and reads the class through ListLogsAt. The
// wrapper lives in this package because the policy belongs to the logs
// domain; the registry stays parameter-free at the listing boundary.
func ListWorkloadLogSources(placement capability.ProviderPlacement, m application.Manifest) ([]observability.SignalSource, error) {
	policy, err := application.LogsPolicy(m)
	if err != nil {
		return nil, err
	}
	return observability.ListLogsAt(placement, []string{m.Name},
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

// ReconcileWorkloadLogSource is the single-service form of
// ReconcileApplicationLogSource for the Loki driver Bind path, which binds one
// concrete workload service at a time.
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
