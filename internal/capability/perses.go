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

// PersesManagementSurfaceContractValue is the Perses management-provider value
// type. It is a purpose-specific management-tool/surface representation,
// deliberately NOT an IntegrationDescriptor and therefore declaring no
// application capability kind (Metrics, Logs, Traces, ...).
//
// The Perses management surface is deliberately NOT an IntegrationDescriptor:
// binding it to a capability kind would supply a capability claim the contract
// forbids, because Perses never IMPLEMENTS a metrics/logs/traces capability on
// its own — it only visualises the surface the backend data owner already ships.
// A real capability kind (Metrics, Logs, Traces, ...) would be a false claim,
// so this contract uses a purpose-specific management-tool/surface type that
// declares no capability kind and carries only what is genuinely true:
//
//   - the Perses management provider kind (PersesProviderKind),
//   - the product version axis, which is DISTINCT from any provider
//     implementation version and any capability specification version
//     (Provider Contract v1: versioning axes). Its value is a realization
//     metadata version, not a capability spec version,
//   - the optional observability management-ui interface that the backend data
//     owner (Prometheus, where the pinned Perses release actually ships the
//     Prometheus plugin) already declares (reuse PersesManagementUIInterface),
//   - the real observability capability owner that Perses visualises: the
//     Prometheus provider kind ("prometheus"), named as an owner reference, NOT
//     as a Perses-declared capability,
//   - the shared placement scope and the optional lifecycle support.
//
// Perses is a provider-owned management/visualization surface, never an
// application capability, so it is never added to ReferenceIntegrations() and
// never appears in the shipped reference provider catalog or in any
// validatable-capability list. It declares NO fake metrics/logs/traces
// application capability of its own.
type PersesManagementSurfaceContractValue struct {
	// Kind is the Perses management provider kind. It is a management surface,
	// never a capability implementation.
	Kind ProviderKind
	// Version is the Perses product realization-version axis. It MUST be a
	// distinct value from the pinned Perses release version (PersesDefaultRelease)
	// and from any capability specification version (Provider Contract v1).
	Version string
	// Owner is the real observability capability owner Perses visualises. It is
	// the Prometheus provider kind, named as an owner reference, NOT as a Perses
	// capability claim.
	Owner ProviderKind
	// Interface is the optional observability management-ui surface Perses
	// realizes, reusing the backend data owner's existing interface.
	Interface ProviderInterface
	// Scope is the shared placement scope of the optional management surface.
	Scope ProviderScope
	// OptionalLifecycle marks the optional status/update/destroy lifecycle this
	// surface exposes.
	OptionalLifecycle bool
}

// PersesManagementSurfaceContract is the exported, package-level Perses
// management-tool/surface contract. It is self-describing and declares no
// application capability kind. It is intentionally NOT an IntegrationDescriptor
// and NOT part of ReferenceIntegrations().
var PersesManagementSurfaceContract = PersesManagementSurfaceContractValue{
	Kind:              PersesProviderKind,
	Version:           "0.1.0",
	Owner:             ProviderPrometheus,
	Interface:         PersesManagementUIInterface,
	Scope:             ScopeShared,
	OptionalLifecycle: true,
}

// OwnerName renders the real observability capability owner Perses visualises as
// a human-readable reference (Prometheus). It exists so consumers can report the
// ownership without asserting a Perses-declared capability.
func (c PersesManagementSurfaceContractValue) OwnerName() string {
	return string(c.Owner)
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
//
// The declaration check is deliberately release-only. Operator authorization is
// a separate question with its own fail-closed verdict; see
// PersesConfiguredStatusNow, which ANDs the two so no status path can report a
// configured surface that is not actually protected.
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

// PersesConfiguredStatusNow is the fail-closed variant of PersesStatusNow: a
// surface is "configured" only when the pinned release validates AND its
// effective security policy validates. An anonymous or unrestricted surface is
// never configured, so the public status cannot advertise management access
// that would in fact be an unauthenticated datasource proxy.
func PersesConfiguredStatusNow(selected bool, policy PersesSecurityPolicy) *PersesStatus {
	status := PersesStatusNotConfigured
	if selected && ValidatePersesSecurityPolicy(policy) == nil {
		status = PersesStatusConfiguredUnverified
	}
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

// hostProviderMetricsNativeVerified reports that the shipped reference provider
// catalog declares a real, native, backend-verified host/provider metrics
// signal (the "collector-metrics" observability signal). It is the witness the
// host/container dashboards require so a positive availability verdict is never
// self-confirmed by the same manifest declaration it is meant to establish.
func hostProviderMetricsNativeVerified() bool {
	for _, descriptor := range ReferenceIntegrations() {
		for _, signal := range descriptor.Observability.Signals {
			if signal.Name != "collector-metrics" {
				continue
			}
			return signal.Kind == ObservabilityMetrics &&
				signal.Status == ObservabilitySupported &&
				signal.Mode == ObservabilityNative &&
				signal.Verification == ObservabilityVerifyBackend
		}
	}
	return false
}

// ResolveHostDashboardSignalStatus maps the manifest truth to the authoritative
// resolved observability-signal verdict the Perses host/container dashboards
// consume. It is a read-only resolver: it never runs a backend probe and never
// builds the enable-path resolver (that is a parallel owner task, 0428-080).
//
// Fail closed on BOTH axes. The host dashboard is realized as "supported" only
// when:
//   - the manifest authoritatively declares the observability backend selected
//     AND the Perses surface pinned (observabilityManagementUI && persesSelected
//     and the pinned Perses release validates); AND
//   - the shipped reference provider catalog declares a real, native,
//     backend-verified host/provider metrics signal (hostProviderMetricsNativeVerified).
//
// Any missing axis resolves to the empty status, which the dashboard reports
// as unavailable-preserving.
func ResolveHostDashboardSignalStatus(observabilityManagementUI bool, persesSelected bool) ObservabilitySupportStatus {
	if !observabilityManagementUI || !persesSelected {
		return ""
	}
	if ValidatePersesRelease(PersesDefaultRelease) != nil {
		return ""
	}
	if !hostProviderMetricsNativeVerified() {
		return ""
	}
	return ObservabilitySupported
}
