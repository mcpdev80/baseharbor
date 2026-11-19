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

// projectTokenFromID extracts the deployment environment of an application
// provider ID/target that carries a canonical project name. The environment is
// the segment that follows the KNOWN application name in the project: the source
// carries its owning application (OwnerApplication) and the runtime operator
// project name in its Target ("runtime://<project>/<service>") or ID ("<provider>:<project>"),
// so the application is the fixed anchor and the remainder after it is the
// environment. This resolves the hyphen-application ambiguity structurally:
// 'bh-prod-my-app-dev' with OwnerApplication 'my-app' anchors on '-my-app-' and
// yields the trailing environment 'dev'; a name with no anchor, more than one
// anchor, or an empty remainder fails closed so it stays invisible at every
// caller.
func projectTokenFromID(source SignalSource) (string, bool) {
	for _, candidate := range []string{source.ID, source.Target} {
		if env, derived := trailingProjectEnvironment(source, candidate); derived {
			return env, true
		}
	}
	return "", false
}

// trailingProjectEnvironment isolates the deployment environment of a canonical
// application project name, tolerating a "<provider>:" identity prefix and a
// "runtime://<project>/<service>" target. The application is the known anchor
// (source.OwnerApplication); the environment is exactly the segment that follows
// the first (and only) '-<application>-' occurrence in the project name. A name
// with no anchor, with more than one anchor, or with an empty remainder after
// the anchor fails closed so the caller fails closed instead of leaking; a
// reserved-platform project (for example 'bh-local-prometheus') fails closed
// because the anchor never matches the application.
func trailingProjectEnvironment(source SignalSource, value string) (string, bool) {
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
	app := strings.ToLower(strings.TrimSpace(source.OwnerApplication))
	if app == "" {
		// No owning application to anchor on — the project name cannot be tied
		// to one environment and fails closed.
		return "", false
	}
	anchor := "-" + app + "-"
	// Count anchors so a name holding the application name twice is ambiguous.
	anchorCount := strings.Count(value, anchor)
	if anchorCount == 0 {
		return "", false
	}
	idx := strings.Index(value, anchor)
	if idx < 0 {
		return "", false
	}
	// The environment is the remainder after the anchor, trimmed of any stray
	// leading/trailing hyphens.
	environment := strings.Trim(strings.ToLower(value[idx+len(anchor):]), "-")
	if environment == "" {
		// Anchor at the very end of the name (for example 'prod-west' with
		// OwnerApplication 'prod-west') — no environment follows, so fail closed.
		return "", false
	}
	if anchorCount > 1 {
		// The application appears more than once — unresolvable, fail closed.
		return "", false
	}
	return environmentToken(environment)
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
