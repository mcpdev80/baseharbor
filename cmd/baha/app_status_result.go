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
	HostContainersDashboards map[string]metricsprovider.PersesHostDashboardAvailabilityEntry `json:"host_containers_dashboards,omitempty"`
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
	resolved, err := resolveApplication(ctx, store, args, "status")
	if err != nil {
		return applicationStatusResult{}, err
	}
	// The Perses three-state contract is manifest-declaration truth, exposed on
	// every status JSON — local AND remote-target faces — regardless of the
	// applied/stopped/not_applied runtime face: not_configured |
	// configured_unverified | verified. Pin validation alone can never map into
	// verified — only a real Perses health probe could. Computed before the
	// remote-target early return below so remote-status --json also carries it.
	persesStatus := capability.PersesStatusNow(resolved.Manifest.Services.ObservabilityManagementUI && resolved.Manifest.Services.Perses)
	resolvedHostSignalStatus := capability.ResolveHostDashboardSignalStatus(
		resolved.Manifest.Services.ObservabilityManagementUI, resolved.Manifest.Services.Perses)
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
	// The remote-target face returns before the local runtime collection: the
	// production status command (app_observe.go) and the MCP status tool route
	// through this collector, and for a remote target collectApplicationStatus
	// (which dials the live Connector session) would fail before the gate fields
	// are ever computed. Build the remote status body here from the resolved
	// manifest (no session dial) and add the same three-state Perses contract and
	// identity-dashboard eligibility the local face emits, so remote-status
	// --json carries them regardless of the applied/stopped/not_applied runtime
	// face. The runtime State stays unverified: a live Core session is required to
	// verify the remote application, but the manifest-declaration gate is not.
	if hasRemoteApplicationTarget(resolved) {
		remoteResult := application.StatusResult{
			ContractVersion: "v1",
			Target:          resolved.Target.Name,
			Application:     resolved.Manifest.Name,
			Environment:     resolved.Manifest.Environment,
			Manifest:        resolved.ManifestPath,
			State:           "unverified",
			Checks:          []application.StatusCheck{},
		}
		remoteResult.AddObservation("remote-provider-project", "unverified", "no owned provider containers observed without a live Core session; complete application absence is unverified")
		remoteResult.AddObservation("application-lifecycle", "unverified", "complete remote application readiness requires workload, protocol, secret and exposure verification")
		return withIdentityDashboardStatus(applicationStatusResult{StatusResult: remoteResult,
			ApplicationID: resolved.Manifest.ApplicationID,
			DeploymentID:  resolved.DeploymentIdentity.DeploymentID,
			OperatorAuth:  collectOperatorAuthObservation(ctx, resolved.Target.Name, resolved.Manifest.Environment),
			HostContainersDashboards: metricsprovider.PersesHostDashboardAvailability(
				resolvedHostSignalStatus),
		}, persesStatus, identityBackendAvailable), nil
	}
	result, err := collectApplicationStatus(ctx, store, args)
	if err != nil {
		return applicationStatusResult{}, err
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
		HostContainersDashboards: metricsprovider.PersesHostDashboardAvailability(resolvedHostSignalStatus),
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
