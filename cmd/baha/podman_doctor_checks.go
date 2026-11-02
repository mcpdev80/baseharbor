package main

import (
	"context"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/health"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// PodmanRuntimeDoctorChecks delegates to the portable runtime
// implementation and converts the result into the health.Check type used by
// the doctor pipeline. The target parameter is accepted for API compatibility
// but is no longer used — podman readiness does not depend on the target
// definition.
func PodmanRuntimeDoctorChecks(_ context.Context, _ deployment.ResolvedTarget) []health.Check {
	cts := bhruntime.PodmanRuntimeDoctorChecks(context.Background())
	checks := make([]health.Check, len(cts))
	for i, c := range cts {
		checks[i] = health.Check{Name: c.Name, OK: c.OK, Message: c.Message}
	}
	return checks
}
