package observability

import (
	"errors"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// Class-application log signal identity and reconcile boundary (card 0428-122).
// These were extracted from registry.go to keep that file within the source
// readability limit while the class-application log writer, its render-time
// reconcile and the Loki driver Bind path share the same registry primitives.

// ApplicationLogSourceID builds the class-application log signal ID of one
// concrete workload service of an application/environment. The ID grammar is
// application:logs:<app>:<env>:<service>, so the service dimension is part of
// the identity: two services of the same application/environment/class are two
// distinguishable sources (docs/architecture/runtime-log-source-contract.md,
// card 0428-122).
func ApplicationLogSourceID(applicationName, environment, service string) string {
	return "application:logs:" + strings.TrimSpace(applicationName) + ":" + strings.TrimSpace(environment) + ":" + strings.TrimSpace(service)
}

// ApplicationLogBoundary is the registry-wide selector for the class-
// application log signals of one application/environment: the ID prefix is
// the single reconciliation key the write side (the render-time reconcile and
// the Loki driver Bind path) and the prune side (PruneApplicationLogs) share,
// mirroring the metrics identity. Exporting it keeps every writer honest
// about the same boundary instead of re-deriving the predicate per call site.
func ApplicationLogBoundary(applicationName, environment string) func(SignalSource) bool {
	prefix := ApplicationLogSourceID(applicationName, environment, "")
	return func(source SignalSource) bool {
		return source.Class == SourceApplication && source.Kind == SignalLogs && strings.HasPrefix(source.ID, prefix)
	}
}

// PruneApplicationLogs reclaims every class-application log signal of one
// application and environment except the IDs the caller keeps. It is the logs
// twin of PruneApplicationMetrics and keeps the listing and the delete
// boundary identical, so a workload log source is never kept or dropped by a
// different key than the one that wrote it.
func PruneApplicationLogs(applicationName, environment string, keepIDs map[string]struct{}) error {
	applicationName = strings.TrimSpace(applicationName)
	environment = strings.TrimSpace(environment)
	if applicationName == "" || environment == "" {
		return errors.New("application log pruning requires application and environment")
	}
	return pruneApplicationSignals(ApplicationLogSourceID(applicationName, environment, ""), keepIDs, func(source SignalSource) bool {
		return source.Class == SourceApplication && source.Kind == SignalLogs
	})
}

// pruneApplicationSignals is the shared prune boundary for a class-
// application signal space: it reclaims every source under one
// application/environment ID prefix except the IDs the caller keeps, so the
// delete side of every path uses exactly the key the write side used
// (card 0428-122). An empty prefix or a nil selector is a caller error, not a
// wildcard.
func pruneApplicationSignals(prefix string, keepIDs map[string]struct{}, owned func(SignalSource) bool) error {
	if !strings.HasSuffix(strings.TrimSpace(prefix), ":") {
		return errors.New("application signal pruning requires an application/environment identity prefix")
	}
	if owned == nil {
		return errors.New("application signal pruning requires its class selector")
	}
	path, err := registryPath()
	if err != nil {
		return err
	}
	return mutate(path, func(sources []SignalSource) ([]SignalSource, error) {
		out := sources[:0]
		for _, source := range sources {
			if owned(source) && strings.HasPrefix(source.ID, prefix) {
				if _, keep := keepIDs[source.ID]; !keep {
					continue
				}
			}
			out = append(out, source)
		}
		return out, nil
	})
}

// ListTraces lists trace signal sources across the given applications,
// optionally including application-provider and platform-provider traces for a
// requested environment (card 0428-122). Extracted alongside the class-
// application log primitives to keep registry.go within the source
// readability limit.
func ListTraces(placement capability.ProviderPlacement, applications []string, includeApplicationProviders, includePlatformProviders bool, requestedEnvironment string) ([]SignalSource, error) {
	return List(SignalTraces, placement, applications, false, includeApplicationProviders, includePlatformProviders, requestedEnvironment)
}
