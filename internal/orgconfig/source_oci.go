package orgconfig

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// OrgRegistryOptInEnv is the single, explicit opt-in switch that lets a
// topology which genuinely needs a shared endpoint resolve the organization
// OCI source against the configured enterprise registry endpoint. It defaults
// OFF: when unset or false the OCI pull path is byte-for-byte behaviourally
// identical to the pre-opt-in baseline, and every other source kind is
// untouched. See the red-first cells in orgconfig_source_oci_test.go.
const OrgRegistryOptInEnv = "BASEHARBOR_ORG_OCI_ENABLED"

// ErrOrgSourceRegistryDisabled is the explicit hard-rejection identity an
// organization OCI source carries when the operator has not opted in through
// OrgRegistryOptInEnv. It documents that the OCI org-config path is opt-in
// only: a caller that does not turn the switch on never reaches the shared
// endpoint, and the error is never downgraded to a mutable fallback. It is
// distinct from ErrSourceUnavailable so no caller can mistake a missing opt-in
// for a merely-unavailable host.
var ErrOrgSourceRegistryDisabled = errors.New("organization OCI source requires explicit opt-in via BASEHARBOR_ORG_OCI_ENABLED")

// ErrOrgSourceRegistryDefault is the distinct hard-rejection identity for a
// configured OCI location that is a public docker.io default rather than an
// enterprise registry. Like the disabled sentinel it is never the
// merely-unavailable class a development opt-in fallback could ever cover.
var ErrOrgSourceRegistryDefault = errors.New("organization OCI source is a public default, not an enterprise registry")

// errOrgSourceRegistryDefaultAlias names the public-default rejection the
// classifier checks.
var errOrgSourceRegistryDefaultAlias = ErrOrgSourceRegistryDefault

// orgRegistryOptInEnabled reports whether the explicit opt-in switch is on.
// It reads the real environment; tests may override it via
// setOrgRegistryOptInForTest. An empty or non-"true" value keeps the
// byte-for-byte baseline behaviour.
func orgRegistryOptInEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(OrgRegistryOptInEnv)), "true")
}

// isHardRegistryRejection reports whether an error in the cause chain is one of
// the distinct hard-rejection identities for the opt-in OCI path: a missing
// opt-in or a public-default endpoint. These are never the merely-unavailable
// class the fail-closed safeguard leaves in force, so they can never be
// covered by a mutable fallback. The identity check is deliberate: an alias of
// the unavailable identity (mutant or misplacement) must not hide itself.
func isHardRegistryRejection(err error) bool {
	return errors.Is(err, errOrgSourceRegistryDefaultAlias) ||
		errors.Is(err, ErrOrgSourceRegistryDisabled)
}

// classifyOrgRegistryError wraps a failure from the opt-in OCI resolve/pull
// with the public-default identity when the configured location is a public
// docker.io default, so the rejection is explicit and can never be downgraded
// into the merely-unavailable class. The default-host check is independent of
// the native tool: a host that merely has oras installed cannot turn a
// public-default endpoint into the unavailable class.
func classifyOrgRegistryError(err error, location string) error {
	if err == nil {
		// Even with a nil underlying failure the configured location can still be
		// a public docker.io default; classify that before returning nil so the
		// public-default rejection is never masked by a nil cause.
		if isDefaultDockerHostLocation(location) {
			return fmt.Errorf("%w: %w: configured endpoint %q is a public default, not an enterprise registry",
				ErrSourceUnavailable, ErrOrgSourceRegistryDefault, location)
		}
		return nil
	}
	if isDefaultDockerHostLocation(location) {
		return fmt.Errorf("%w: %w: configured endpoint %q is a public default, not an enterprise registry",
			ErrSourceUnavailable, ErrOrgSourceRegistryDefault, location)
	}
	return err
}

// IsNativeToolNotFound reports whether a native artifact-tool failure carries
// the executable-file-not-found identity (possibly wrapped by an exec-error
// chain), the one tool failure that means the host simply lacks the oras
// binary rather than being unreachable. It is deliberately narrower than a
// substring match on a redacted message: only this identity may ever be
// reported as a missing oras installation, so an alias that hid it inside the
// unavailable class would still fail the classification tests.
func IsNativeToolNotFound(err error) bool {
	return errors.Is(err, exec.ErrNotFound)
}

// ErrNativeToolNotFound is the actionable absence identity for a host that
// simply lacks the native OCI artifact tool (oras). It unwraps to the
// merely-unavailable class so every existing gate keeps its semantics, and
// the identity itself is what callers classify on — never a generic "oras:
// command failed". An oras that exists but fails for another reason (network,
// denial) never carries this identity.
var ErrNativeToolNotFound = fmt.Errorf("%w: the native OCI artifact tool 'oras' is not installed; install ORAS or provide the artifact through the container runtime", ErrSourceUnavailable)

// classifyNativeToolFailure maps a raw exec failure from a native artifact
// tool either to the actionable missing-oras identity or to the redacted
// generic class. The raw stderr is never surfaced.
func classifyNativeToolFailure(name string, err error) error {
	if IsNativeToolNotFound(err) {
		return ErrNativeToolNotFound
	}
	return fmt.Errorf("%s: command failed", name)
}

// isDefaultDockerHostLocation reports whether a configured OCI location is a
// public docker.io default (registry.docker.io, index.docker.io or the bare
// docker.io namespace), never an enterprise registry. A public default is a
// hard rejection on the opt-in path: resolving it would mask a missing
// configuration or a denied endpoint and could silently pull from the public
// default.
func isDefaultDockerHostLocation(location string) bool {
	host := strings.TrimSpace(location)
	if host == "" {
		return false
	}
	host = strings.ToLower(host)
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	// Strip an explicit path segment so only the registry host is judged.
	if i := strings.IndexAny(host, "/@:"); i >= 0 {
		host = host[:i]
	}
	switch host {
	case "docker.io", "registry.docker.io", "index.docker.io", "registry-1.docker.io", "":
		return true
	}
	return false
}
