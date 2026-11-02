package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"time"
)

// Check is the health check result type used by the doctor pipeline.
// It is declared in this package to avoid import cycles:
//
//	internal/runtime → internal/health → internal/providers/runtime/podman → internal/runtime
//
// By inlining the struct definition here, the runtime package no longer
// depends on internal/health.
type Check struct {
	Name    string
	OK      bool
	Message string
}

// PodmanRuntimeDoctorChecks probes the Podman runtime and Quadlet generator for
// readiness and returns the two health checks the native-run acceptance greps
// for ("OK podman", "OK quadlet"). The control-plane doctor path intentionally
// drops the legacy health.Doctor() runtime probes, which never emitted a
// named runtime check; emitting them here keeps a Podman target's doctor able
// to report READY once Podman+Quadlet are available while leaving Docker-target
// doctor output untouched (Docker assertions require every check ready and
// never expect a podman check).
func PodmanRuntimeDoctorChecks(ctx context.Context) []Check {
	checks := make([]Check, 0, 2)
	checks = append(checks, podmanEngineDoctorCheck(ctx))
	checks = append(checks, quadletGeneratorDoctorCheck(ctx))
	return checks
}

func podmanEngineDoctorCheck(ctx context.Context) Check {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, err := exec.LookPath("podman"); err != nil {
		return Check{Name: "podman", OK: false, Message: "podman executable not found: " + err.Error()}
	}
	if authorityErr := verifyLocalRuntimeAuthority(ctx); authorityErr != nil {
		return Check{Name: "podman", OK: false, Message: "selected Podman engine is not a verified native rootless Linux authority: " + authorityErr.Error()}
	}
	return Check{Name: "podman", OK: true, Message: "native rootless Podman engine reachable"}
}

// verifyLocalRuntimeAuthority probes podman info JSON to confirm that the
// selected engine is a verified native rootless Linux authority. This is the
// same check the podman provider uses (VerifyLocalRuntimeAuthority), but
// inlined here so the portable doctor path does not import the concrete
// provider package.
func verifyLocalRuntimeAuthority(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	path, err := exec.LookPath("podman")
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, path, "info", "--format", "json")
	cmd.Env = runtimeCommandEnv(path)
	data, err := cmd.Output()
	if err != nil {
		return errors.New("selected Podman engine authority could not be observed")
	}
	return verifyLocalAuthorityInfo(data)
}

func verifyLocalAuthorityInfo(data []byte) error {
	var info struct {
		Host struct {
			OS       string `json:"os"`
			Remote   *bool  `json:"serviceIsRemote"`
			Security struct {
				Rootless *bool `json:"rootless"`
			} `json:"security"`
		} `json:"host"`
	}
	if len(data) > 1<<20 || json.Unmarshal(data, &info) != nil || info.Host.OS != "linux" || info.Host.Remote == nil || *info.Host.Remote || info.Host.Security.Rootless == nil || !*info.Host.Security.Rootless {
		return errors.New("selected Podman engine is not a verified native rootless Linux authority")
	}
	return nil
}

func quadletGeneratorDoctorCheck(ctx context.Context) Check {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if !quadletAvailable(ctx) {
		return Check{Name: "quadlet", OK: false, Message: "Podman Quadlet generator unavailable"}
	}
	return Check{Name: "quadlet", OK: true, Message: "Podman Quadlet generator available"}
}

// quadletAvailable reports whether this host can run rootless Podman Quadlet
// units. Inlined here to avoid importing the concrete provider package, which
// would create an import cycle with internal/runtime.
func quadletAvailable(ctx context.Context) bool {
	if _, err := quadletGeneratorPath(); err != nil {
		return false
	}
	path, err := exec.LookPath("podman")
	if err != nil {
		return false
	}
	cmd := exec.CommandContext(ctx, path, "version")
	cmd.Env = runtimeCommandEnv(path)
	return cmd.Run() == nil
}

// quadletGeneratorPath resolves the path to the Podman Quadlet generator
// binary, checking standard locations.
func quadletGeneratorPath() (string, error) {
	if path, err := exec.LookPath("podman-system-generator"); err == nil {
		return path, nil
	}
	for _, path := range []string{
		"/usr/lib/systemd/system-generators/podman-system-generator",
		"/usr/libexec/podman/quadlet",
	} {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return path, nil
		}
	}
	return "", errors.New("Podman Quadlet generator not found")
}
