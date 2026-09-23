package main

import (
	"context"
	"errors"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func classifyOperationalFailure(err error, resource string) error {
	if err == nil { return nil }
	var typed *machine.Error
	if errors.As(err, &typed) {
		if typed.Resource == "" { typed.Resource = resource }
		return typed
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &machine.Error{Code: machine.ErrorTimeout, Message: "The operation timed out before readiness was verified.", Resource: resource, Remediation: "manual/admin action required", Next: "Check runtime/provider health and retry with --verbose if the cause is not obvious.", Retryable: true, Cause: err}
	}
	lower := strings.ToLower(err.Error())
	switch {
	case bhruntime.IsPortBindingConflict(err):
		return &machine.Error{Code: machine.ErrorPortConflict, Message: "A published host port is already in use.", Resource: resource, Remediation: "requires developer input", Next: "Free the conflicting port or make the Compose host binding configurable with ${VAR:-PORT}.", Cause: err}
	case strings.Contains(lower, "pull access denied"), strings.Contains(lower, "manifest unknown"), strings.Contains(lower, "failed to pull"), strings.Contains(lower, "unable to pull"), strings.Contains(lower, "image not known"):
		return &machine.Error{Code: machine.ErrorImagePullFailed, Message: "The required container image could not be pulled.", Resource: resource, Remediation: "requires developer input", Next: "Verify the image name/tag and registry access, then retry.", Cause: err}
	case strings.Contains(lower, "unauthorized"), strings.Contains(lower, "authentication required"), strings.Contains(lower, "permission denied"), strings.Contains(lower, "forbidden"):
		return &machine.Error{Code: machine.ErrorAuthenticationFailed, Message: "Authentication or authorization failed.", Resource: resource, Remediation: "requires developer input", Next: "Verify the configured credentials/permissions and retry.", Cause: err}
	case strings.Contains(lower, "cannot connect to the docker daemon"), strings.Contains(lower, "cannot connect to podman"), strings.Contains(lower, "podman socket"), strings.Contains(lower, "docker daemon is not running"), strings.Contains(lower, "runtime unavailable"):
		return &machine.Error{Code: machine.ErrorRuntimeUnavailable, Message: "The container runtime is unavailable.", Resource: resource, Remediation: "manual/admin action required", Next: "Start or repair the selected Docker/Podman runtime and retry.", Retryable: true, Cause: err}
	case strings.Contains(lower, "unsupported capability"), strings.Contains(lower, "missing runtime capability"), strings.Contains(lower, "required runtime capability"):
		return &machine.Error{Code: machine.ErrorCapabilityMissing, Message: "A required runtime/provider capability is unavailable.", Resource: resource, Remediation: "manual/admin action required", Next: "Use a compatible runtime/provider or enable the required capability.", Cause: err}
	case strings.Contains(lower, "compose") && (strings.Contains(lower, "invalid") || strings.Contains(lower, "config") || strings.Contains(lower, "yaml")):
		return &machine.Error{Code: machine.ErrorInvalidWorkload, Message: "The application workload configuration is invalid.", Resource: resource, Remediation: "requires developer input", Next: "Fix the reported Compose/workload configuration and retry.", Cause: err}
	default:
		return &machine.Error{Code: machine.ErrorWorkloadStartFailed, Message: "The application workload failed to start.", Resource: resource, Remediation: "requires developer input", Next: "Run with --verbose for the underlying runtime error and fix the reported workload failure.", Cause: err}
	}
}
