package health

import (
	"context"

	podmanprovider "github.com/mcpdev80/baseharbor/internal/providers/runtime/podman"
)

// PodmanRuntimeDoctorChecks converts provider readiness into doctor checks.
func PodmanRuntimeDoctorChecks(ctx context.Context) []Check {
	providerChecks := podmanprovider.RuntimeDoctorChecks(ctx)
	checks := make([]Check, len(providerChecks))
	for i, check := range providerChecks {
		checks[i] = Check{Name: check.Name, OK: check.OK, Message: check.Message}
	}
	return checks
}
