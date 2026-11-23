package main

import (
	"context"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/availability"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
)

type runtimeArtifactObservation struct {
	Reference       string `json:"reference,omitempty"`
	ImageID         string `json:"image_id,omitempty"`
	Digest          string `json:"digest,omitempty"`
	ExpectedVersion string `json:"expected_version,omitempty"`
	Detail          string `json:"detail,omitempty"`
}

type applicationStatusResult struct {
	application.StatusResult
	ApplicationID            string                                                          `json:"application_id"`
	DeploymentID             string                                                          `json:"deployment_id"`
	TLS                      *applicationTLSObservation                                      `json:"tls,omitempty"`
	ServiceTLS               []application.BackendTLSLifecycleObservation                    `json:"service_tls,omitempty"`
	RuntimeArtifact          *runtimeArtifactObservation                                     `json:"runtime_artifact,omitempty"`
	RuntimeDocsURL           string                                                          `json:"runtime_docs_url,omitempty"`
	ApplicationURLs          []string                                                        `json:"application_urls,omitempty"`
	ManagementUI             []application.ManagementUISurface                               `json:"management_ui,omitempty"`
	OperatorAuth             operatorAuthObservation                                         `json:"operator_auth"`
	Availability             []availability.NegotiationResult                                `json:"availability,omitempty"`
	Perses                   *capability.PersesStatus                                        `json:"perses,omitempty"`
	DatabaseDashboards       map[string]metricsprovider.PersesDashboardAvailabilityEntry     `json:"database_dashboards,omitempty"`
	HostContainersDashboards map[string]metricsprovider.PersesHostDashboardAvailabilityEntry `json:"host_containers_dashboards,omitempty"`
	// PersesBackendWitness carries, per observability backend, the manifest-only
	// three-state backend-witness (configured / not-configured / unreachable) so
	// the operator sees which source is green or red instead of a blanket
	// "unavailable". It is manifest declaration truth: it never asserts a
	// "healthy"/"verified" backend from a manifest flag. See befund-1.md.
	PersesBackendWitness []persesBackendSignalVerdict `json:"perses_backend_witness,omitempty"`
	// InfrastructureDashboards is the panel-inventory map that the r9 witness
	// commit (67658ef86) dropped from the struct and both setters. Card 0428-142
	// requires BOTH the manifest-only backend witness above and this panel
	// inventory in the same result: the witness names which source is declared
	// (configured / not-configured / unreachable), the inventory names each
	// infra panel (cpu/memory/disk/network) with its verdict. Restoring the
	// field and its json-tag here keeps the contract that
	// TestBahaStatusInfrastructureDashboardsExposesEveryPanel asserts; the
	// producer is set below in both status paths.
	InfrastructureDashboards map[string]metricsprovider.PersesInfraDashboardAvailabilityEntry `json:"infrastructure_dashboards,omitempty"`
	// Versions is the unified version dimension of this deployment (card
	// 0428-062): the separated wanted/applied/observed cells from the delivered
	// read model plus the component rows with their source-model provenance.
	// VersionWarnings names why a dimension or row could not be answered.
	Versions        appStatusVersions `json:"versions"`
	VersionWarnings []string          `json:"version_warnings,omitempty"`
	// IdentityDashboardEligible is the non-forgeable availability of the
	// Keycloak identity dashboard: eligible ONLY when a real Keycloak native
	// metrics signal exists for this Target, the identity provider is actually
	// selected, and the Keycloak placement is a valid, BaseHarbor-owned
	// authorization (fail-closed). Derived from IdentitySelectionWitness via
	// NonForgeableAvailable() — never a caller-supplied status boolean.
	IdentityDashboardEligible bool `json:"identity_dashboard_eligible,omitempty"`

	tlsStatus     *applicationTLSStatus
	tlsErr        error
	serviceTLSErr error
}

// persesBackendSignal is the three-state vocabulary the card forbids
// over-claiming with: it names only what manifest truth can decide, never a
// proven backend. A real backend probe (Loki/Tempo Verification=none,
// reference.go:133-134, :147-148) is card 0428-143, not this one.
type persesBackendSignal string

const (
	// persesBackendNotConfigured: the observability signal is absent from the manifest.
	persesBackendNotConfigured persesBackendSignal = "not-configured"
	// persesBackendConfigured: the observability signal is declared in the manifest
	// (collected / exported), but the backend's own answer has not been observed.
	persesBackendConfigured persesBackendSignal = "configured"
	// persesBackendUnreachable: declared but the backend's own answer is absent.
	persesBackendUnreachable persesBackendSignal = "unreachable"
)

// persesBackendSignalVerdict is one backend's manifest-only witness.
type persesBackendSignalVerdict struct {
	Backend  string              `json:"backend"`
	SourceID string              `json:"source_id"`
	State    persesBackendSignal `json:"state"`
	Reason   string              `json:"reason,omitempty"`
}

// persesBackendSignalWitnessFromManifest derives the per-backend three-state
// witness from a real resolved backend signal, never from a manifest flag alone.
// Every backend answers from its OWN signal and its OWN reachability:
//   - Prometheus from the resolved host/provider-metrics observability signal
//     (ResolveHostDashboardSignalStatus): "supported" means declared AND the
//     backend's own signal resolved; otherwise the declared backend is
//     "unreachable" (declared but the backend's own answer is absent).
//   - Loki from its own logs-collection reachability, Tempo from its own
//     OTLP-traces reachability (metricsprovider.PersesBackendSignalWitness).
//
// The three states are fail-closed and never over-claim:
//   - "not-configured": the signal is absent from the manifest.
//   - "configured": the signal is declared AND resolved reachable (reached AND
//     matched); the backend's own answer was observed.
//   - "unreachable": the signal is declared but NOT resolved reachable — the
//     backend's own answer is absent. This is the real emission path the card
//     mandates; it is derived from the resolved signal, never from a manifest
//     selection flag.
//
// It never returns "verified"/"healthy". A real backend health probe
// (Loki/Tempo Verification=none, reference.go:133-134, :147-148) is card
// 0428-143, not this one. This is the single witness shared by the human
// `perses status` face and the machine `app status` host-containers JSON so the
// two faces never drift (card 0428-142).
func persesBackendSignalWitnessFromManifest(m application.Manifest, resolvedHostSignalStatus capability.ObservabilitySupportStatus, witness metricsprovider.PersesBackendSignalWitness) []persesBackendSignalVerdict {
	observabilitySelected := m.Services.ObservabilityManagementUI && m.Services.Perses
	if !observabilitySelected {
		return nil
	}
	var witnessVerdicts []persesBackendSignalVerdict
	// Prometheus (metrics): declared in the manifest, reachable only when the
	// resolved host/provider-metrics observability signal is supported AND a
	// real backend answer is proven. A positive "configured" must never be
	// claimed from the static capability catalog alone: the shipped reference
	// catalog marks metrics Verification=none (card 0428-143 makes no
	// verification assertion for that source), so a supported signal without a
	// non-empty Prometheus provenance is unreachable, not configured. This is
	// the (b) boundary CODEX-REVIEW r5 left pending.
	if application.HasMetricsSources(m) {
		if resolvedHostSignalStatus == capability.ObservabilitySupported && witness.PrometheusProven {
			witnessVerdicts = append(witnessVerdicts, persesBackendSignalVerdict{
				Backend:  "prometheus",
				SourceID: "metrics.sources",
				State:    persesBackendConfigured,
				Reason:   "Prometheus metrics signal declared (metrics.sources) and a real Prometheus provider-metric answer is proven reachable; the resolved signal being supported with an empty witness is unreachable, not configured (card 0428-143 owns the non-empty witness)",
			})
		} else {
			witnessVerdicts = append(witnessVerdicts, persesBackendSignalVerdict{
				Backend:  "prometheus",
				SourceID: "metrics.sources",
				State:    persesBackendUnreachable,
				Reason:   "Prometheus metrics signal declared (metrics.sources) but the resolved host/provider-metrics observability signal is not proven reachable (no real Prometheus provider-metric answer); the prometheus/dashboard is explicitly unreachable",
			})
		}
	} else {
		witnessVerdicts = append(witnessVerdicts, persesBackendSignalVerdict{
			Backend:  "prometheus",
			SourceID: "metrics.sources",
			State:    persesBackendNotConfigured,
			Reason:   "no metrics source declared in the manifest; the prometheus/dashboard is explicitly unavailable",
		})
	}
	// Loki (logs): declared in the manifest, reachable only when the manifest's
	// own logs collection resolves and its backend answer is present.
	if application.HasLogsCollection(m) {
		if witness.LokiProven {
			witnessVerdicts = append(witnessVerdicts, persesBackendSignalVerdict{
				Backend:  "loki",
				SourceID: "logs.collect",
				State:    persesBackendConfigured,
				Reason:   "Loki logs signal declared (logs.collect) and the Loki provider-log stream is reachable; a real health probe is card 0428-143, not a manifest flag",
			})
		} else {
			witnessVerdicts = append(witnessVerdicts, persesBackendSignalVerdict{
				Backend:  "loki",
				SourceID: "logs.collect",
				State:    persesBackendUnreachable,
				Reason:   "Loki logs signal declared (logs.collect) but no verified Loki provider-log stream signal is present; the loki/dashboard is explicitly unreachable",
			})
		}
	} else {
		witnessVerdicts = append(witnessVerdicts, persesBackendSignalVerdict{
			Backend:  "loki",
			SourceID: "logs.collect",
			State:    persesBackendNotConfigured,
			Reason:   "no logs collection declared in the manifest; the loki/dashboard is explicitly unavailable",
		})
	}
	// Tempo (traces): declared in the manifest, reachable only when the
	// manifest's own OTLP traces export resolves and its backend answer is
	// present.
	if application.HasOTLPTelemetry(m) {
		if witness.TempoProven {
			witnessVerdicts = append(witnessVerdicts, persesBackendSignalVerdict{
				Backend:  "tempo",
				SourceID: "telemetry.otlp",
				State:    persesBackendConfigured,
				Reason:   "Tempo traces signal declared (telemetry.otlp) and the Tempo provider-trace signal is reachable; a real health probe is card 0428-143, not a manifest flag",
			})
		} else {
			witnessVerdicts = append(witnessVerdicts, persesBackendSignalVerdict{
				Backend:  "tempo",
				SourceID: "telemetry.otlp",
				State:    persesBackendUnreachable,
				Reason:   "Tempo traces signal declared (telemetry.otlp) but no verified Tempo provider-trace signal is present; the tempo/dashboard is explicitly unreachable",
			})
		}
	} else {
		witnessVerdicts = append(witnessVerdicts, persesBackendSignalVerdict{
			Backend:  "tempo",
			SourceID: "telemetry.otlp",
			State:    persesBackendNotConfigured,
			Reason:   "no OTLP traces export declared in the manifest; the tempo/dashboard is explicitly unavailable",
		})
	}
	return witnessVerdicts
}

func canonicalDevelopmentManagementSurfaces(resolved resolvedApplication, surfaces []application.ManagementUISurface) []application.ManagementUISurface {
	result := append([]application.ManagementUISurface(nil), surfaces...)
	for i := range result {
		var host string
		var err error
		service := result[i].Service
		if service == "rabbitmq" || strings.HasPrefix(service, "rabbitmq-") || service == "mongodb" || strings.HasPrefix(service, "mongodb-") {
			host, err = devaccess.ApplicationHost(resolved.Target.Name, resolved.Manifest.Name, service)
		} else {
			switch service {
			case "sql":
				placement, placementErr := application.ResolveProviderPlacement(resolved.Manifest, capability.ProviderPostgreSQL)
				if placementErr != nil {
					continue
				}
				if placement.Scope == capability.ScopeShared {
					host, err = devaccess.SharedHost(resolved.Target.Name, "pgadmin")
				} else {
					host, err = devaccess.ApplicationHost(resolved.Target.Name, resolved.Manifest.Name, "pgadmin")
				}
			case "cache", "key-value":
				placement, placementErr := application.ResolveProviderPlacement(resolved.Manifest, capability.ProviderValkey)
				if placementErr != nil {
					continue
				}
				if placement.Scope == capability.ScopeShared {
					host, err = devaccess.SharedHost(resolved.Target.Name, "cache")
				} else {
					host, err = devaccess.ApplicationHost(resolved.Target.Name, resolved.Manifest.Name, "cache")
				}
			case "object-storage":
				placement, placementErr := application.ResolveProviderPlacement(resolved.Manifest, capability.ProviderSeaweedFS)
				if placementErr != nil {
					continue
				}
				if placement.Scope == capability.ScopeShared {
					host, err = devaccess.SharedHost(resolved.Target.Name, "storage")
				} else if placement.Scope == capability.ScopeApplication {
					host, err = devaccess.ApplicationHost(resolved.Target.Name, resolved.Manifest.Name, "storage")
				}
			case "secrets":
				host, err = devaccess.SharedHost(resolved.Target.Name, "openbao")
			case "identity", "identity-login":
				placement, placementErr := application.ResolveProviderPlacement(resolved.Manifest, capability.ProviderKeycloak)
				if placementErr != nil {
					continue
				}
				if placement.Scope == capability.ScopeShared {
					host, err = devaccess.SharedHost(resolved.Target.Name, "identity")
				} else if placement.Scope == capability.ScopeApplication {
					host, err = devaccess.ApplicationHost(resolved.Target.Name, resolved.Manifest.Name, "identity")
				}
			case "identity-admin":
				placement, placementErr := application.ResolveProviderPlacement(resolved.Manifest, capability.ProviderKeycloak)
				if placementErr != nil {
					continue
				}
				if placement.Scope == capability.ScopeShared {
					host, err = devaccess.SharedHost(resolved.Target.Name, "identity-admin")
				} else if placement.Scope == capability.ScopeApplication {
					host, err = devaccess.ApplicationHost(resolved.Target.Name, resolved.Manifest.Name, "identity-admin")
				}
			case "observability":
				placement, placementErr := metricsprovider.PlacementForAt(resolved.TargetStateRoot, resolved.Target.Name, resolved.Manifest)
				if placementErr != nil {
					continue
				}
				if placement.Scope == capability.ScopeShared {
					host, err = devaccess.SharedHost(resolved.Target.Name, "prometheus")
				} else if placement.Scope == capability.ScopeApplication {
					host, err = devaccess.ApplicationHost(resolved.Target.Name, resolved.Manifest.Name, "prometheus")
				}
			default:
				continue
			}
		}
		if err == nil && strings.TrimSpace(host) != "" {
			result[i].URL = devgateway.URLForTarget(resolved.Target.Name, host)
		}
	}
	return result
}

func collectApplicationStatusResult(ctx context.Context, store application.Store, args []string) (applicationStatusResult, error) {
	result, err := collectApplicationStatus(ctx, store, args)
	if err != nil {
		return applicationStatusResult{}, err
	}
	resolved, err := resolveApplication(ctx, store, args, "status")
	if err != nil {
		return applicationStatusResult{}, err
	}
	// The Perses three-state contract is manifest-declaration truth, exposed on
	// every status JSON — local AND remote-target faces — regardless of the
	// applied/stopped/not_applied runtime face: not_configured |
	// configured_unverified | verified. Pin validation alone can never map into
	// verified — only the real per-target health probe (metrics
	// VerifyPersesInfraDashboard) can. Computed before the remote-target early
	// return below so remote-status --json also carries it. The probe runs
	// against this resolved manifest's selected Perses surface and materialized
	// Prometheus sources: an answering target promotes to verified (so the
	// 0428-078 identity_dashboard_eligible=true case becomes reachable), a
	// silent/not-probed target fails closed to configured_unverified.
	persesStatus := metricsprovider.PersesBackendAnswersStatusNow(ctx, resolved.Manifest)
	resolvedHostSignalStatus := capability.ResolveHostDashboardSignalStatus(
		resolved.Manifest.Services.ObservabilityManagementUI, resolved.Manifest.Services.Perses)
	infrastructureDashboardStatus := metricsprovider.PersesInfraDashboardAvailability(ctx, resolved.Manifest)
	databaseDashboards := metricsprovider.PersesDashboardAvailability(resolved.Manifest)
	// The Keycloak identity dashboard realizes the identity provider, so its
	// availability is a non-forgeable decision: it is eligible ONLY when the
	// Perses dashboard is actually selected AND verified (PersesStatusNow maps
	// manifest truth to not_configured | configured_unverified | verified, and
	// a real Perses health probe is required to reach verified), a real
	// Keycloak native metrics signal exists for this Target, the identity
	// provider is actually selected, and the Keycloak placement is a valid,
	// BaseHarbor-owned authorization for this Target (fail-closed). The
	// predicate is verified-only: both not_configured and configured_unverified
	// fail closed — configured_unverified means no real Perses health path
	// exists, so eligibility must never be emitted for that state. Resolve the
	// witness from real primitives before the remote-target early return so the
	// gate is evaluated on both local and remote-target faces. An eligible
	// identity-dashboard status can NEVER be emitted while Perses is
	// not_configured or configured_unverified.
	identityBackendAvailable := identityDashboardBackendEligible(
		persesStatus,
		resolved.Manifest,
		resolved.TargetStateRoot,
		resolved.Target.Name,
	)
	if hasRemoteApplicationTarget(resolved) {
		return withIdentityDashboardStatus(applicationStatusResult{StatusResult: result,
			ApplicationID:      resolved.Manifest.ApplicationID,
			DeploymentID:       resolved.DeploymentIdentity.DeploymentID,
			OperatorAuth:       collectOperatorAuthObservation(ctx, resolved.Target.Name, resolved.Manifest.Environment),
			Perses:             persesStatus,
			DatabaseDashboards: databaseDashboards,
			HostContainersDashboards: metricsprovider.PersesHostDashboardAvailability(
				resolvedHostSignalStatus),
			PersesBackendWitness:     persesBackendSignalWitnessFromManifest(resolved.Manifest, resolvedHostSignalStatus, metricsprovider.PersesBackendSignalWitness{}),
			InfrastructureDashboards: infrastructureDashboardStatus}, persesStatus, identityBackendAvailable), nil
	}
	tlsStatus, tlsObservation, tlsErr := collectApplicationTLSObservation(resolved)
	if tlsErr != nil || (tlsObservation != nil && !tlsObservation.Healthy) {
		result.Ready = false
	}

	var serviceTLS []application.BackendTLSLifecycleObservation
	var serviceTLSErr error
	if result.State != "not_applied" {
		if files, filesErr := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest); filesErr == nil {
			serviceTLS, serviceTLSErr = application.InspectBackendTLSLifecycle(files, resolved.Manifest)
			if serviceTLSErr != nil || !serviceTLSLifecycleHealthy(serviceTLS) {
				result.Ready = false
			}
		}
	}

	var managementUI []application.ManagementUISurface
	if result.State != "not_applied" {
		if files, filesErr := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest); filesErr == nil {
			managementUI, _, _ = application.ApplicationManagementUISurfaces(resolved.Manifest, files)
		}
		if identityUI, identityErr := identityprovider.KeycloakManagementSurfaces(resolved.Manifest, resolved.TargetStateRoot, resolved.Target.Name); identityErr == nil {
			managementUI = append(managementUI, identityUI...)
		}
		if resolved.Manifest.Services.ObjectStorageManagementUI {
			if surface, surfaceErr := objectstorage.ManagementUISurfaceAt(resolved.TargetStateRoot, resolved.Target.Name); surfaceErr == nil {
				managementUI = append(managementUI, surface)
			}
		}
		if resolved.Manifest.Services.SecretsManagementUI {
			if platformFiles, platformErr := existingTargetRuntimeFiles(ctx); platformErr == nil {
				if surface, surfaceErr := openBaoManagementUISurface(platformFiles); surfaceErr == nil {
					managementUI = append(managementUI, surface)
				}
			}
		}
		if resolved.Manifest.Services.ObservabilityManagementUI {
			if surface, surfaceErr := metricsprovider.ManagementUISurfaceAt(resolved.TargetStateRoot, resolved.Target.Name, resolved.Manifest); surfaceErr == nil {
				managementUI = append(managementUI, surface)
			}
		}
		if devaccess.Enabled(resolved.Manifest.Environment) {
			managementUI = canonicalDevelopmentManagementSurfaces(resolved, managementUI)
		}
	}

	var availabilityResults []availability.NegotiationResult
	if runtimeProvider, runtimeErr := detectRuntimeForTarget(ctx, resolved.Target); runtimeErr == nil {
		resolution, availabilityErr := application.ResolveAvailability(resolved.Manifest, string(runtimeProvider.Kind()), runtimeProvider.Descriptor().Availability)
		availabilityResults = resolution.Results
		if availabilityErr != nil {
			result.Ready = false
		}
	} else if resolved.Manifest.HA {
		result.Ready = false
	}

	var runtimeArtifact *runtimeArtifactObservation
	var runtimeDocsURL string
	if application.RequiresRuntimeBroker(resolved.Manifest) && result.State != "not_applied" {
		if files, filesErr := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest); filesErr == nil {
			if brokerFiles, brokerErr := runtimebroker.Existing(files); brokerErr == nil {
				runtimeDocsURL = strings.TrimSpace(brokerFiles.DocsURL)
				if requiresDevelopmentGateway(resolved.Manifest) && runtimeDocsURL != "" {
					if host, hostErr := devaccess.ApplicationHost(resolved.Target.Name, resolved.Manifest.Name, "api"); hostErr == nil {
						runtimeDocsURL = devgateway.URLForTarget(resolved.Target.Name, host) + "/swagger/"
					}
				}
				runtimeArtifact = &runtimeArtifactObservation{
					Reference:       strings.TrimSpace(brokerFiles.Image),
					ExpectedVersion: strings.TrimSpace(version),
				}
				if runtimeProvider, runtimeErr := detectRuntimeForTarget(ctx, resolved.Target); runtimeErr == nil {
					identity, identityErr := runtimeProvider.ProjectServiceImageIdentity(ctx, runtimebroker.ProjectNameForRuntime(resolved.Manifest, files), runtimebroker.ServiceName)
					if identityErr != nil {
						runtimeArtifact.Detail = identityErr.Error()
					} else {
						runtimeArtifact.Reference = identity.Reference
						runtimeArtifact.ImageID = identity.ImageID
						runtimeArtifact.Digest = identity.Digest
					}
				} else {
					runtimeArtifact.Detail = runtimeErr.Error()
				}
			}
		}
	}
	versions, versionWarnings := collectAppStatusVersions(resolved)
	return withIdentityDashboardStatus(applicationStatusResult{
		StatusResult:             result,
		ApplicationID:            resolved.Manifest.ApplicationID,
		DeploymentID:             resolved.DeploymentIdentity.DeploymentID,
		OperatorAuth:             collectOperatorAuthObservation(ctx, resolved.Target.Name, resolved.Manifest.Environment),
		Availability:             availabilityResults,
		TLS:                      tlsObservation,
		ServiceTLS:               serviceTLS,
		RuntimeArtifact:          runtimeArtifact,
		RuntimeDocsURL:           runtimeDocsURL,
		ApplicationURLs:          applicationAccessURLs(resolved, result.Ready),
		ManagementUI:             managementUI,
		Perses:                   persesStatus,
		DatabaseDashboards:       databaseDashboards,
		HostContainersDashboards: metricsprovider.PersesHostDashboardAvailability(resolvedHostSignalStatus),
		PersesBackendWitness:     persesBackendSignalWitnessFromManifest(resolved.Manifest, resolvedHostSignalStatus, metricsprovider.PersesBackendSignalWitness{}),
		InfrastructureDashboards: infrastructureDashboardStatus,
		Versions:                 versions,
		VersionWarnings:          versionWarnings,
		tlsStatus:                tlsStatus,
		tlsErr:                   tlsErr,
		serviceTLSErr:            serviceTLSErr,
	}, persesStatus, identityBackendAvailable), nil
}

// withIdentityDashboardStatus binds the same verified-only decision to both
// local and remote public status results.
func withIdentityDashboardStatus(result applicationStatusResult, persesStatus *capability.PersesStatus, eligible bool) applicationStatusResult {
	result.Perses = persesStatus
	result.IdentityDashboardEligible = eligible
	return result
}

// identityDashboardBackendEligible is the verified-only, non-forgeable decision
// the identity-dashboard gate publishes on BOTH status faces (the local return
// and the remote-target early return share this one call). It is eligible ONLY
// when:
//
//   - the Perses dashboard is verified (persesStatus == verified). PersesStatusNow
//     maps manifest truth to not_configured | configured_unverified | verified;
//     verified requires a real Perses health probe, so a selected, pinned
//     surface without that probe stays configured_unverified and fails closed.
//     Both not_configured and configured_unverified therefore fail closed.
//
//   - a real, current Keycloak native metrics signal exists for this concrete
//     Target (MetricCurrent), the identity provider is actually selected, and
//     the Keycloak placement is a valid, BaseHarbor-owned authorization for this
//     Target (fail-closed). This is IdentitySelectionWitnessAt().NonForgeableAvailable(),
//     which binds the signal to this Target's binding so a foreign Target's
//     signal on the same sharing boundary cannot forge it.
//
// It is the single validated entrance for the identity-dashboard availability
// flag on the status JSON; a caller can never obtain true by supplying a
// verified status alone without the real witness (fail-closed on both faces).
func identityDashboardBackendEligible(persesStatus *capability.PersesStatus, m application.Manifest, targetStateRoot, targetName string) bool {
	// verified-only: not_configured and configured_unverified fail closed.
	if persesStatus == nil || *persesStatus != capability.PersesStatusVerified {
		return false
	}
	keycloakPlacement, keycloakErr := application.ResolveProviderPlacement(m, capability.ProviderKeycloak)
	if keycloakErr != nil {
		return false
	}
	identityWitness := metricsprovider.IdentitySelectionWitnessAt(m, keycloakPlacement, targetStateRoot, targetName)
	return identityWitness.NonForgeableAvailable()
}
