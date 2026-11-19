package observability

import "strings"

// environment.go — route-2 (card 0428-121) environment derivation. These
// helpers resolve the deployment environment of a registry entry from the
// source's own persisted data only; no caller passes an environment and the
// capability ProviderPlacement carries none on this base, so the dimension must
// live on the source. SourceMatchesEnvironment in registry.go compares the
// resolved value against the caller's requested environment.

// SourceEnvironment derives the environment dimension of a registry entry from
// the source's OWN data only (route 2 / caller-agnostic): no caller passes an
// environment and the capability ProviderPlacement carries none on this base,
// so the environment must live on the source.
//
//   - application-scoped metrics (SourceApplication, SignalMetrics) carry an
//     explicit environment in the canonical application-metrics ID grammar
//     (application:metrics:<application>:<environment>:<source>); the middle
//     segment is the environment.
//   - application-scoped providers (SourceApplicationProvider, e.g.
//     loki/tempo/prometheus of one application) carry the deployment project
//     name (bh-<target>-<application>-<environment>) in their ID or target; the
//     trailing hyphen token is the environment.
//   - shared/platform sources carry their environment in the sharing boundary
//     ("environment:<env>").
//
// The boolean is false when no environment resolves. For an application-scoped
// source that is the fail-closed case the reviewer requires: a source without
// an explicit or unambiguously derivable environment must not appear in any
// single-environment result instead of leaking into another environment.
// Shared/platform sources with an unresolvable boundary stay environment-agnostic.
func SourceEnvironment(source SignalSource) (environment string, ok bool) {
	// application-scoped metrics: explicit environment segment in the ID grammar.
	if source.Class == SourceApplication && source.Kind == SignalMetrics {
		if env, derived := applicationMetricsSourceEnvironment(source.ID); derived {
			return env, true
		}
	}
	// application-scoped providers: trailing project token of the project name.
	if source.Class == SourceApplicationProvider {
		if env, derived := projectTokenFromID(source); derived {
			return env, true
		}
	}
	// shared/platform: environment:<env> sharing boundary.
	if env, derived := sharingBoundaryEnvironment(source); derived {
		return env, true
	}
	return "", false
}

// applicationMetricsSourceEnvironment returns the environment segment of the
// canonical application-metrics ID grammar (application:metrics:<app>:<env>:<src>).
func applicationMetricsSourceEnvironment(id string) (string, bool) {
	const prefix = "application:metrics:"
	if !strings.HasPrefix(id, prefix) {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(id, prefix), ":")
	if len(parts) != 3 {
		return "", false
	}
	env := strings.TrimSpace(parts[1])
	if env == "" {
		return "", false
	}
	return env, true
}

// projectTokenFromID extracts the trailing environment token of an
// application provider ID/target that carries a canonical project name
// (bh-<target>-<app>-<env>) or a "<provider>:" identity prefix / "runtime://<project>/<svc>" target.
func projectTokenFromID(source SignalSource) (string, bool) {
	for _, candidate := range []string{source.ID, source.Target} {
		if env, derived := trailingProjectEnvironment(candidate); derived {
			return env, true
		}
	}
	return "", false
}

// trailingProjectEnvironment isolates the environment token of a canonical
// application project name, tolerating a "<provider>:" identity prefix and a
// "runtime://<project>/<service>" target. A hyphenated trailing token (for
// example the "west" of a legacy "prod-west" project) is ambiguous and yields
// ok=false so the caller fails closed instead of leaking.
func trailingProjectEnvironment(value string) (string, bool) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "runtime://")
	if strings.Contains(value, "/") {
		// runtime://<project>/<service> — isolate the project segment first.
		value = value[:strings.Index(value, "/")]
	}
	if idx := strings.Index(value, ":"); idx >= 0 {
		// "<provider>:<project>" (or "id:project") — keep only the first
		// colon-delimited segment, which carries the project name.
		value = value[:idx]
	}
	// Runtime project name: baseharbor-<target>-<app>-<env> (4 tokens,
	// namespaced) or baseharbor-<app>-<env> (3 tokens, namespace-less).
	if strings.HasPrefix(value, "baseharbor-") {
		value = strings.TrimPrefix(value, "baseharbor-")
		parts := strings.Split(value, "-")
		// baseharbor-<app>-<env> => two remaining tokens, or
		// baseharbor-<target>-<app>-<env> => three remaining tokens.
		if len(parts) == 2 || len(parts) == 3 {
			return environmentToken(parts[len(parts)-1])
		}
		return "", false
	}
	// Operator project name: bh-<target>-<app>-<env> (3 tokens after "bh-").
	value = strings.TrimPrefix(value, "bh-")
	parts := strings.Split(value, "-")
	if len(parts) != 3 {
		return "", false
	}
	return environmentToken(parts[2])
}

// environmentToken normalises a trailing project token into an environment:
// empty, "shared", or a reserved platform token fails closed (ambiguous).
func environmentToken(token string) (string, bool) {
	env := strings.ToLower(strings.TrimSpace(token))
	if env == "" || env == "shared" || reservedEnvironmentToken(env) {
		return "", false
	}
	return env, true
}

// sharingBoundaryEnvironment reads the environment:<env> sharing boundary of a
// shared/platform source.
func sharingBoundaryEnvironment(source SignalSource) (string, bool) {
	boundary := strings.TrimSpace(source.SharingBoundary)
	const prefix = "environment:"
	if !strings.HasPrefix(boundary, prefix) {
		return "", false
	}
	env := strings.TrimSpace(strings.TrimPrefix(boundary, prefix))
	if env == "" {
		return "", false
	}
	return strings.ToLower(env), true
}

// reservedEnvironmentToken marks project tokens that are shared platform
// resources rather than one application's deployment environment.
func reservedEnvironmentToken(value string) bool {
	switch value {
	case "shared", "postgres", "openbao", "runtime-executor", "object-storage",
		"metrics", "prometheus", "logs", "loki", "traces", "tempo",
		"telemetry", "otel-collector", "seaweedfs":
		return true
	default:
		return false
	}
}
