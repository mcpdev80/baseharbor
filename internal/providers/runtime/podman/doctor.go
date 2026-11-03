package podman

import (
	"context"
	"os/exec"
	"time"
)

// DoctorCheck describes the readiness of a Podman runtime component.
type DoctorCheck struct {
	Name    string
	OK      bool
	Message string
}

// RuntimeDoctorChecks checks the selected Podman authority and Quadlet support.
func RuntimeDoctorChecks(ctx context.Context) []DoctorCheck {
	return []DoctorCheck{podmanEngineDoctorCheck(ctx), quadletGeneratorDoctorCheck(ctx)}
}

func podmanEngineDoctorCheck(ctx context.Context) DoctorCheck {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, err := exec.LookPath("podman"); err != nil {
		return DoctorCheck{Name: "podman", OK: false, Message: "podman executable not found: " + err.Error()}
	}
	if err := (Provider{}).VerifyLocalRuntimeAuthority(ctx); err != nil {
		return DoctorCheck{Name: "podman", OK: false, Message: "selected Podman engine is not a verified native rootless Linux authority: " + err.Error()}
	}
	return DoctorCheck{Name: "podman", OK: true, Message: "native rootless Podman engine reachable"}
}

func quadletGeneratorDoctorCheck(ctx context.Context) DoctorCheck {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if !QuadletAvailable(ctx) {
		return DoctorCheck{Name: "quadlet", OK: false, Message: "Podman Quadlet generator unavailable"}
	}
	return DoctorCheck{Name: "quadlet", OK: true, Message: "Podman Quadlet generator available"}
}
