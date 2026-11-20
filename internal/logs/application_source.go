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

// applicationLogCanonicalService is the fallback service token of a
// class-application log signal whose caller names no concrete workload
// service. It is the one entry the registry holds for the bare workload
// identity - never a stand-in for the deployment's services: the established
// runtime log identity keeps the service dimension in the ID, so each
// workload service of an application/environment is its own source
// (docs/architecture/runtime-log-source-contract.md, card 0428-122).
const applicationLogCanonicalService = "workload"

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

// applicationLogSource builds the class-application log signal of one
// concrete workload service of a deployment and validates it against the
// registry contract. The service dimension belongs in the ID: two services of
// the same application/environment, class and provider are two distinguishable
// sources, exactly as the runtime log source contract documents (card
// 0428-122 correction round).
func applicationLogSource(m application.Manifest, project, service string) (observability.SignalSource, error) {
	service = strings.TrimSpace(service)
	if service == "" {
		service = applicationLogCanonicalService
	}
	if strings.TrimSpace(project) == "" {
		return observability.SignalSource{}, fmt.Errorf("application log source %q requires its runtime project", service)
	}
	source := observability.SignalSource{
		ID:                 observability.ApplicationLogSourceID(m.Name, m.Environment, service),
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

// applicationLogSourceSet is the writer-side of the service-carrying identity:
// it turns the caller's service names into the desired class-application log
// signals of this deployment, without duplicates and in the order the caller
// named them. An empty or blank service list keeps the documented behaviour of
// the bare workload identity: exactly the canonical "workload" entry.
func applicationLogSourceSet(m application.Manifest, project string, services []string) ([]observability.SignalSource, error) {
	names := deduplicateServiceNames(services)
	if len(names) == 0 {
		names = []string{applicationLogCanonicalService}
	}
	desired := make([]observability.SignalSource, 0, len(names))
	for _, service := range names {
		source, err := applicationLogSource(m, project, service)
		if err != nil {
			return nil, err
		}
		desired = append(desired, source)
	}
	return desired, nil
}

// applicationLogBoundary is the local mirror of observability.ApplicationLogBoundary
// for this package's writers: the reconciliation boundary of the class-
// application log signals of one application/environment. Every writer of this
// class - the whole-set reconcile, the per-service Bind registration and the
// render-time reconciliation in internal/application - goes through it, so no
// call site re-derives its own predicate over the registry (card 0428-122).
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
// driver Bind path land here. The registry identity is the workload service
// identity: one class-application log signal per application/environment AND
// service, whose ID and Target both name that service. The reconciliation
// boundary is the application/environment class space, so the desired set the
// caller passes registers the ACTUALLY active services and reclaim removes the
// entries of services that are no longer desired - stale keys are never merely
// appended to, and newly appearing services are never swallowed. A disabled
// deployment policy reclaims every signal of the boundary: the registry never
// keeps a source the deployment stopped collecting.
func ReconcileApplicationLogSource(m application.Manifest, project string, services []string) error {
	policy, err := application.LogsPolicy(m)
	if err != nil {
		return err
	}
	desired := []observability.SignalSource{}
	if policy.Enabled && policy.Collect[application.LogsSourceApplication] {
		// An enabled policy writes exactly the desired set: one signal per
		// distinct service the caller names (the canonical workload entry when
		// it names none). Repeated reconciles replace, never accumulate.
		desired, err = applicationLogSourceSet(m, project, services)
		if err != nil {
			return err
		}
	}
	return observability.ReconcileMatching(applicationLogBoundary(m), desired)
}

// deduplicateServiceNames keeps the first occurrence of every non-blank
// service name, trimmed. Repeated renders or repeated Bind calls for the same
// service must not produce duplicate desired entries.
func deduplicateServiceNames(services []string) []string {
	seen := make(map[string]struct{}, len(services))
	out := make([]string, 0, len(services))
	for _, service := range services {
		service = strings.TrimSpace(service)
		if service == "" {
			continue
		}
		if _, ok := seen[service]; ok {
			continue
		}
		seen[service] = struct{}{}
		out = append(out, service)
	}
	return out
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
		policy.Enabled && policy.Collect[application.LogsSourceApplication], false, false, m.Environment)
}

// ConvergeManagedLogsForReconcile is the collector-side entry of the
// application-class log writer for a deployment whose workload services the
// caller does not enumerate: it registers the canonical workload identity.
// Callers that know the deployment's services use ReconcileApplicationLogSource
// with that set, which is what the render-time reconciliation in
// internal/application does.
func ConvergeManagedLogsForReconcile(m application.Manifest, project string) error {
	return ReconcileApplicationLogSource(m, project, nil)
}

// ReconcileWorkloadLogSource registers exactly one workload service's class-
// application log signal - the same identity ReconcileApplicationLogSource
// writes, reached directly by the Loki driver Bind path, which binds one
// concrete service at a time. It reconciles only its own ID: every other
// class-application source of this application/environment keeps its entry, so
// binding a second service never overwrites the first one's signal. An
// enabled policy writes exactly one signal per service; a disabled policy
// fails closed here because reclaim is the reconciliation entry's job, not the
// bind path's.
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
	// The Bind path owns one service, so it reconciles exactly one ID: the
	// predicate selects that entry inside the application/environment class
	// space and leaves every sibling service's source in place. Replacing the
	// whole space here is what overwrote the first service when a second one
	// was bound (card 0428-122 correction round).
	return observability.ReconcileMatching(func(existing observability.SignalSource) bool {
		return applicationLogBoundary(m)(existing) && existing.ID == source.ID
	}, []observability.SignalSource{source})
}
