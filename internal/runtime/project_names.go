package runtime

import "strings"

// SharedProjectName returns the operator-visible Compose project for target-wide
// BaseHarbor platform services. Empty namespace means the implicit local target.
func SharedProjectName(namespace string) string {
	return "bh-" + projectToken(namespace, "local") + "-shared"
}

// SharedResourceProjectName returns the stable physical resource prefix for
// target-wide platform resources. It is intentionally independent from the
// operator-visible consolidated Compose project name.
func SharedResourceProjectName(namespace string) string {
	return "baseharbor-" + projectToken(namespace, "local")
}

func sharedResourceProjectNameForOperatorProject(project string) string {
	project = strings.TrimSpace(project)
	if strings.HasPrefix(project, "bh-") && strings.HasSuffix(project, "-shared") {
		target := strings.TrimSuffix(strings.TrimPrefix(project, "bh-"), "-shared")
		if target != "" {
			return "baseharbor-" + target
		}
	}
	return project
}

// ApplicationProjectName returns the operator-visible Compose project for one
// application on a target. Empty namespace means the implicit local target.
func ApplicationProjectName(namespace, application, environment string) string {
	target := projectToken(namespace, "local")
	app := projectToken(application, "app")
	env := projectToken(environment, "dev")
	if reservedApplicationProjectToken(app) {
		app += "-app"
	}
	return "bh-" + target + "-" + app + "-" + env
}

// TargetInstallationProject is the shared identity installation project the
// identityprovider producer registers its observability signal under for one
// concrete Target: SharedProjectName("<target>-core"). The Target attribute
// travels in the canonical runtime Target's SERVICE component
// (internal/identityprovider keycloakSignalTargetAttribution), never here:
// projectToken maps dots to hyphens, so the valid names "a.b" and "a-b"
// produce the SAME value and this grammar is NOT injective in the Target.
func TargetInstallationProject(target string) string {
	return SharedProjectName(strings.TrimSpace(target) + "-core")
}

func projectToken(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, ".", "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return fallback
	}
	return value
}

func reservedApplicationProjectToken(value string) bool {
	switch value {
	case "shared", "postgres", "openbao", "runtime-executor", "object-storage",
		"metrics", "prometheus", "logs", "loki", "traces", "tempo",
		"telemetry", "otel-collector", "seaweedfs":
		return true
	default:
		return false
	}
}
