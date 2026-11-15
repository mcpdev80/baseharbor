package capability

import (
	"fmt"
	"strings"
)

// Perses management-provider seam.
//
// Perses is an OPTIONAL, provider-owned management/visualization surface. It is
// NOT an application capability and NOT a required BaseHarbor Core bundle. It is
// a realization of the existing optional observability management-ui interface
// that the backend data owners (Prometheus, and Loki/Tempo where the pinned
// Perses release actually ships and supports their plugin) already declare on
// their integration descriptor.
//
// This file only declares the contract: the optional Perses management tool,
// the version axes that stay independent, the accepted interfaces/scopes/
// ownership/lifecycle/auth, and the fail-closed "not configured" rule. It does
// NOT implement container lifecycle: that belongs to a reviewed contract before
// downstream changes.
//
// Never false-declare metrics/logs/traces capabilities. A surface that cannot
// be safely integrated MUST be reported as unsupported, never as configured.

// PersesProviderKind identifies the optional Perses management tool. It is a
// management surface, not a capability implementation, so it has no portable
// application service.
const PersesProviderKind ProviderKind = "perses"

// PersesManagementUIInterface is the optional realization of the observability
// management-ui surface by Perses. It reuses the existing observability
// management-ui interface class and protocol; it is never intrinsic because the
// backend data owners ship their own native management-ui as well.
var PersesManagementUIInterface = ProviderInterface{
	Name:     "management-ui",
	Class:    InterfaceObservability,
	Protocol: "https",
	Optional: true,
}

// PersesManagement is the optional Perses management-provider descriptor.
//
// Its product version and image digest are realization metadata (Provider
// Contract v1: "Provider identity and versioning"); they MUST NOT replace the
// provider implementation version or the capability specification version.
var PersesManagement = IntegrationDescriptor{
	ID:              "baseharbor/perses",
	Version:         "0.1.0",
	Protocol:        ProviderProtocolV1,
	Provider:        Provider{Kind: PersesProviderKind, Capabilities: []Kind{}},
	Services:        []ServiceKind{ServiceObservability},
	SupportedScopes: []ProviderScope{ScopeShared, ScopeApplication},
	Interfaces:      []ProviderInterface{PersesManagementUIInterface},
}

// PersesRelease is the pinned Perses release this Core references as the
// optional management realization. It is product/version realization metadata
// and MUST NOT be exposed as a provider implementation version or a capability
// specification version.
type PersesRelease struct {
	// Version is the pinned Perses product version (e.g. "v0.55.1").
	Version string
	// Image is the pinned production image reference (e.g.
	// "persesdev/perses:v0.55.1" or an OCI digest).
	Image string
	// PrometheusPluginVersion is the pinned Prometheus datasource plugin
	// version shipped in the selected candidate.
	PrometheusPluginVersion string
	// AuthenticationClass is the effective auth class this surface reports when
	// Perses is selected. It maps to exactly one of the management
	// authentication class strings below; it defaults to unsupported until
	// authentication is explicitly configured (Perses is opt-in).
	AuthenticationClass string
}

const (
	persesAuthUnsupported      = "unsupported"
	persesAuthNativeCredential = "native-credential"
	persesAuthStandardsAdapter = "standards-auth-adapter"
	persesAuthNativeOIDC       = "native-oidc"
)

// PersesDefaultRelease is the pinned candidate for v0.4.28. It must be a
// release whose security fixes are current: v0.55.0+ (JWT/SSRF/datasource-proxy
// fixes) with an encryption key mandatory when authentication is enabled.
var PersesDefaultRelease = PersesRelease{
	Version:                 "v0.55.1",
	Image:                   "persesdev/perses:v0.55.1",
	PrometheusPluginVersion: "v0.6.0",
	AuthenticationClass:     persesAuthUnsupported,
}

// ValidatePersesRelease pins the release contract for the optional management
// surface. It is fail-closed: without a pinned product version and image the
// surface is not configured, never "configured".
func ValidatePersesRelease(release PersesRelease) error {
	if strings.TrimSpace(release.Version) == "" || strings.TrimSpace(release.Image) == "" {
		return &PersesNotConfiguredError{}
	}
	switch release.AuthenticationClass {
	case persesAuthUnsupported, persesAuthNativeCredential, persesAuthStandardsAdapter, persesAuthNativeOIDC:
	default:
		return fmt.Errorf("Perses management surface has unsupported authentication class %q", release.AuthenticationClass)
	}
	return nil
}

// PersesNotConfiguredError is returned when the optional Perses management
// surface is requested but not provisioned with a pinned release. It is the
// fail-closed "not configured" state, not a misconfiguration.
type PersesNotConfiguredError struct{}

func (*PersesNotConfiguredError) Error() string {
	return "Perses management surface selected but not configured: pin a Perses release (product version + image) and provision it; without it the observability management UI is unsupported, not configured"
}

// PersesManagementSurface is the declarative Perses realization of the
// observability management-ui surface. Perses is a provider-owned management
// tool, never an application capability.
type PersesManagementSurface struct {
	Release                 string
	Image                   string
	PrometheusPluginVersion string
	AuthenticationClass     string
	// Provisioned is grounded in manifest truth: the surface is selected
	// (services.perses) and the pinned release contract validates. It reports
	// whether the surface is configured by declaration, NOT whether a Perses
	// container is running. Fail closed: without selection or a pinned release
	// it is false (not-configured), never a false "configured" declaration.
	Provisioned bool
	// Reachability is a SEPARATE axis from Provisioned. Provisioned can be true
	// while Reachability is only "unverified": this Core defines the optional
	// management seam only and ships no Perses runtime health probe, so the
	// surface can never report "verified"/"healthy". Never true without an
	// actual reachable Perses health path (there is none in this task).
	Reachability string
}

// PersesReachabilityUnverified is the only reachability state this Core reports
// for the optional Perses management surface: the contract is defined and
// fail-closed, but no Perses runtime health probe exists here, so the surface
// can never assert "verified"/"healthy". No SUPPORTED HA line can be claimed.
const PersesReachabilityUnverified = "unverified"

// PersesStatus is the machine-readable public status state of the optional
// Perses management surface. It is EXACTLY one of the three values below and
// no other: the status contract exposes the three distinguishable states to
// consumers, not the internal Provisioned/Reachability axes.
//
//   - "not_configured":    manifest without a selected Perses surface.
//   - "configured_unverified": selected AND pinned release validated, but no
//     real Perses health probe exists, so reachability stays unverified. This
//     is the ONLY reachable state this Core produces for a selected surface.
//   - "verified":          only when a real Perses health probe observes the
//     surface. That probe does not exist in this Core, so this state can never
//     be produced here — pin validation alone MUST NOT emit it.
type PersesStatus string

const (
	PersesStatusNotConfigured        PersesStatus = "not_configured"
	PersesStatusConfiguredUnverified PersesStatus = "configured_unverified"
	PersesStatusVerified             PersesStatus = "verified"
)

// PersesStatusNotConfiguredPtr is the pointer form of
// PersesStatusNotConfigured, used where a *PersesStatus is required (e.g. the
// fail-closed early returns of the management-UI surface builder).
var PersesStatusNotConfiguredPtr = persesStatusNotConfiguredInstance()

func persesStatusNotConfiguredInstance() *PersesStatus {
	status := PersesStatusNotConfigured
	return &status
}

// PersesStatusNow maps the manifest truth to the public three-state contract:
//   - unselected surface            -> not_configured
//   - selected + pinned release OK  -> configured_unverified (always, because no
//     real Perses health path exists to prove reachability)
//   - selected but release invalid  -> not_configured (fail-closed)
//
// It never returns "verified": a real Perses health probe is required for that,
// and this Core defines the optional seam only. Pin validation alone must never
// map into "verified"/"ready".
func PersesStatusNow(selected bool) *PersesStatus {
	if !selected {
		status := PersesStatusNotConfigured
		return &status
	}
	if ValidatePersesRelease(PersesDefaultRelease) != nil {
		status := PersesStatusNotConfigured
		return &status
	}
	status := PersesStatusConfiguredUnverified
	return &status
}

// PersesManagementSurfaceNow returns the declarative Perses realization for the
// selected optional observability management surface. Fail closed: it is
// grounded in manifest truth — selected (services.perses) and the pinned
// release contract validates — and reports Provisioned=true only then, as a
// "configured by declaration" surface. Reachability is always unverified: this
// Core defines the seam, it does not run a Perses health probe. Never a false
// "configured"/"healthy" declaration.
func PersesManagementSurfaceNow(selected bool) PersesManagementSurface {
	release := PersesDefaultRelease
	surface := PersesManagementSurface{
		Release:                 release.Version,
		Image:                   release.Image,
		PrometheusPluginVersion: release.PrometheusPluginVersion,
		AuthenticationClass:     release.AuthenticationClass,
		Provisioned:             false,
		Reachability:            PersesReachabilityUnverified,
	}
	if selected && ValidatePersesRelease(release) == nil {
		// A pinned release is a declaration contract, not a running container.
		// Provisioning (image pulled, project reconciled) is verified at
		// runtime; here the surface is only "configured by declaration".
		surface.Provisioned = true
	}
	return surface
}

// AuthClass returns the management authentication class this surface reports,
// mapped to the shared management-access vocabulary (native-oidc,
// standards-auth-adapter, native-credential, unsupported).
func (s PersesManagementSurface) AuthClass() string {
	return s.AuthenticationClass
}

// PersesManagementSurfaceUnprovisioned is the fail-closed surface returned when
// Perses is selected but no release is pinned: Provisioned=false, reachability
// unverified, auth unsupported.
func PersesManagementSurfaceUnprovisioned() PersesManagementSurface {
	return PersesManagementSurface{
		Release:                 PersesDefaultRelease.Version,
		Image:                   PersesDefaultRelease.Image,
		PrometheusPluginVersion: PersesDefaultRelease.PrometheusPluginVersion,
		AuthenticationClass:     persesAuthUnsupported,
		Provisioned:             false,
		Reachability:            PersesReachabilityUnverified,
	}
}
