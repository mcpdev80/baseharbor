package main

import (
	"context"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/health"
)

// PodmanRuntimeDoctorChecks reports the Podman provider's readiness.
func PodmanRuntimeDoctorChecks(ctx context.Context, _ deployment.ResolvedTarget) []health.Check {
	return health.PodmanRuntimeDoctorChecks(ctx)
}
