package main

import (
	"context"
	"os/exec"
	"time"

	"github.com/mcpdev80/baseharbor/internal/health"
	podmanprovider "github.com/mcpdev80/baseharbor/internal/providers/runtime/podman"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// PodmanRuntimeDoctorChecks probes the Podman runtime and Quadlet generator for
// the effective Target and returns the two readiness checks the native-run
// acceptance greps for ("OK podman", "OK quadlet"). The control-plane doctor
// path intentionally drops the legacy health.Doctor() runtime probes, which
// never emitted a named runtime check; emitting them here keeps a Podman
// target's doctor able to report READY once Podman+Quadlet are available while
// leaving Docker-target doctor output untouched (Docker assertions require every
// check ready and never expect a podman check).
func PodmanRuntimeDoctorChecks(ctx context.Context, target deployment.ResolvedTarget) []health.Check {
	checks := make([]health.Check, 0, 2)
	checks = append(checks, podmanEngineDoctorCheck(ctx))
	checks = append(checks, quadletGeneratorDoctorCheck(ctx))
	return checks
}

func podmanEngineDoctorCheck(ctx context.Context) health.Check {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, err := exec.LookPath("podman"); err != nil {
		return health.Check{Name: "podman", OK: false, Message: "podman executable not found: " + err.Error()}
	}
	authorityErr := podmanprovider.Provider{}.VerifyLocalRuntimeAuthority(ctx)
	if authorityErr != nil {
		return health.Check{Name: "podman", OK: false, Message: "selected Podman engine is not a verified native rootless Linux authority: " + authorityErr.Error()}
	}
	return health.Check{Name: "podman", OK: true, Message: "native rootless Podman engine reachable"}
}

func quadletGeneratorDoctorCheck(ctx context.Context) health.Check {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if !podmanprovider.QuadletAvailable(ctx) {
		return health.Check{Name: "quadlet", OK: false, Message: "Podman Quadlet generator unavailable"}
	}
	return health.Check{Name: "quadlet", OK: true, Message: "Podman Quadlet generator available"}
}
