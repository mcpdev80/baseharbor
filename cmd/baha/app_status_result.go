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
	// PersesBackendWitness carries an independently observed, fail-closed
	// three-state witness for each declared observability backend. A positive
	// answer comes from that backend's authorized query path, never from a
	// manifest flag; it does not claim general service health.
	PersesBackendWitness []persesBackendSignalVerdict `json:"perses_backend_witness,omitempty"`
	// InfrastructureDashboards is the panel-inventory map that the r9 witness
	// commit (67658ef86) dropped from the struct and both setters. Card 0428-142
	// requires BOTH the live backend witness above and this panel
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

	tlsStatus     *applicationTLSStatus
	tlsErr        error
	serviceTLSErr error
}

// persesBackendSignal is the per-source vocabulary. A positive state requires
// the corresponding backend's own live query to answer; declaration alone
// cannot produce it.
type persesBackendSignal string

const (
	// persesBackendNotConfigured: the observability signal is absent from the manifest.
	persesBackendNotConfigured persesBackendSignal = "not-configured"
	// persesBackendConfigured: the signal is declared and its own backend
	// returned an authorized matching answer.
	persesBackendConfigured persesBackendSignal = "configured"
	// persesBackendUnreachable: declared but the backend's own answer is absent.
	persesBackendUnreachable persesBackendSignal = "unreachable"
)

// persesBackendSignalVerdict is one backend's source witness.
type persesBackendSignalVerdict struct {
	Backend  string              `json:"backend"`
	SourceID string              `json:"source_id"`
	State    persesBackendSignal `json:"state"`
	Reason   string              `json:"reason,omitempty"`
}

// persesBackendSignalWitnessFromManifest combines declared source intent with
// independent backend observations, never from a manifest flag alone.
// Every backend answers from its own source query: Prometheus via its live
// metrics probe, Loki via its authorized log-stream query, and Tempo via its
// authorized TraceQL query.
//
// The three states are fail-closed and never over-claim:
//   - "not-configured": the signal is absent from the manifest.
//   - "configured": the signal is declared and its backend-specific query
//     returned a matching answer.
//   - "unreachable": the signal is declared but its query did not return a
//     matching answer.
//
// It never returns "verified"/"healthy". These observations only prove that
// the source-specific backend query returned a match. This is the single
// witness shared by the human
// `perses status` face and the machine `app status` host-containers JSON so the
// two faces never drift (card 0428-142).
func persesBackendSignalWitnessFromManifest(m application.Manifest, resolvedHostSignalStatus capability.ObservabilitySupportStatus, witness metricsprovider.PersesBackendSignalWitness) []persesBackendSignalVerdict {
	observabilitySelected := m.Services.ObservabilityManagementUI && m.Services.Perses
	if !observabilitySelected {
		return nil
	}
	var witnessVerdicts []persesBackendSignalVerdict
	// Prometheus (metrics): declaration and resolved host/provider-metrics
	// support are necessary, but configured also requires a live matching
	// answer from the Prometheus query path.
	if application.HasMetricsSources(m) {
		if resolvedHostSignalStatus == capability.ObservabilitySupported && witness.PrometheusProven {
			witnessVerdicts = append(witnessVerdicts, persesBackendSignalVerdict{
				Backend:  "prometheus",
				SourceID: "metrics.sources",
				State:    persesBackendConfigured,
				Reason:   "Prometheus metrics source is declared and its provider-metric query returned a live answer",
			})
		} else {
			witnessVerdicts = append(witnessVerdicts, persesBackendSignalVerdict{
				Backend:  "prometheus",
				SourceID: "metrics.sources",
				State:    persesBackendUnreachable,
				Reason:   "Prometheus metrics source is declared but its provider-metric query returned no live answer; the prometheus/dashboard is explicitly unreachable",
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
				Reason:   "Loki logs source is declared and its authorized provider-log query returned a matching stream",
			})
		} else {
			witnessVerdicts = append(witnessVerdicts, persesBackendSignalVerdict{
				Backend:  "loki",
				SourceID: "logs.collect",
				State:    persesBackendUnreachable,
				Reason:   "Loki logs source is declared but its authorized provider-log query returned no matching stream; the loki/dashboard is explicitly unreachable",
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
				Reason:   "Tempo trace source is declared and its authorized TraceQL query returned a matching trace",
			})
		} else {
			witnessVerdicts = append(witnessVerdicts, persesBackendSignalVerdict{
				Backend:  "tempo",
				SourceID: "telemetry.otlp",
				State:    persesBackendUnreachable,
				Reason:   "Tempo trace source is declared but its authorized TraceQL query returned no matching trace; the tempo/dashboard is explicitly unreachable",
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
	// The Perses lifecycle state retains its fail-closed three-state contract.
	// The separate backend witness below is observed per source before the
	// remote-target return, so local and remote JSON surfaces receive the same
	// independently probed Prometheus, Loki and Tempo answers.
	persesStatus := metricsprovider.PersesBackendAnswersStatusNow(ctx, resolved.Manifest)
	persesBackendWitness := observePersesBackendSignalWitness(ctx, resolved.Manifest, persesBackendSignalProbesWithPrometheusStatus(persesStatus))
	resolvedHostSignalStatus := capability.ResolveHostDashboardSignalStatus(
		resolved.Manifest.Services.ObservabilityManagementUI, resolved.Manifest.Services.Perses)
	infrastructureDashboardStatus := metricsprovider.PersesInfraDashboardAvailability(ctx, resolved.Manifest)
	databaseDashboards := metricsprovider.PersesDashboardAvailability(resolved.Manifest)
	earlyVersions, earlyVersionWarnings := collectAppStatusVersions(resolved)
	if hasRemoteApplicationTarget(resolved) {
		return applicationStatusResult{StatusResult: result, ApplicationID: resolved.Manifest.ApplicationID,
			DeploymentID:       resolved.DeploymentIdentity.DeploymentID,
			OperatorAuth:       collectOperatorAuthObservation(ctx, resolved.Target.Name, resolved.Manifest.Environment),
			Perses:             persesStatus,
			DatabaseDashboards: databaseDashboards,
			HostContainersDashboards: metricsprovider.PersesHostDashboardAvailability(
				resolvedHostSignalStatus),
			PersesBackendWitness:     persesBackendSignalWitnessFromManifest(resolved.Manifest, resolvedHostSignalStatus, persesBackendWitness),
			InfrastructureDashboards: infrastructureDashboardStatus,
			Versions:                 earlyVersions,
			VersionWarnings:          earlyVersionWarnings}, nil
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
	return applicationStatusResult{
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
		PersesBackendWitness:     persesBackendSignalWitnessFromManifest(resolved.Manifest, resolvedHostSignalStatus, persesBackendWitness),
		InfrastructureDashboards: infrastructureDashboardStatus,
		Versions:                 versions,
		VersionWarnings:          versionWarnings,
		tlsStatus:                tlsStatus,
		tlsErr:                   tlsErr,
		serviceTLSErr:            serviceTLSErr,
	}, nil
}
