package main

import (
	"context"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
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
	TLS             *applicationTLSObservation                   `json:"tls,omitempty"`
	ServiceTLS      []application.BackendTLSLifecycleObservation `json:"service_tls,omitempty"`
	RuntimeArtifact *runtimeArtifactObservation                  `json:"runtime_artifact,omitempty"`
	RuntimeDocsURL  string                                       `json:"runtime_docs_url,omitempty"`
	ManagementUI    []application.ManagementUISurface            `json:"management_ui,omitempty"`
	OperatorAuth    operatorAuthObservation                      `json:"operator_auth"`

	tlsStatus     *applicationTLSStatus
	tlsErr        error
	serviceTLSErr error
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
			managementUI, _ = application.ApplicationManagementUISurfaces(resolved.Manifest, files)
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
	}

	if devaccess.Enabled(resolved.Manifest.Environment) {
		for i := range managementUI {
			var host string
			var hostErr error
			switch managementUI[i].Service {
			case "sql":
				host, hostErr = devaccess.ApplicationHost(resolved.Target.Name, resolved.Manifest.Name, "pgadmin")
			case "cache":
				host, hostErr = devaccess.ApplicationHost(resolved.Target.Name, resolved.Manifest.Name, "cache")
			case "object-storage":
				placement, placementErr := application.ResolveProviderPlacement(resolved.Manifest, capability.ProviderSeaweedFS)
				if placementErr != nil {
					hostErr = placementErr
				} else if placement.Scope == capability.ScopeShared {
					host, hostErr = devaccess.SharedHost(resolved.Target.Name, "storage")
				} else if placement.Scope == capability.ScopeApplication {
					host, hostErr = devaccess.ApplicationHost(resolved.Target.Name, resolved.Manifest.Name, "storage")
				}
			case "secrets":
				host, hostErr = devaccess.SharedHost(resolved.Target.Name, "openbao")
			case "identity-login":
				host, hostErr = devaccess.ApplicationHost(resolved.Target.Name, resolved.Manifest.Name, "identity")
			case "identity-admin":
				host, hostErr = devaccess.ApplicationHost(resolved.Target.Name, resolved.Manifest.Name, "identity-admin")
			case "observability":
				placement, placementErr := metricsprovider.PlacementForAt(resolved.TargetStateRoot, resolved.Target.Name, resolved.Manifest)
				if placementErr != nil {
					hostErr = placementErr
				} else if placement.Scope == capability.ScopeShared {
					host, hostErr = devaccess.SharedHost(resolved.Target.Name, "prometheus")
				} else if placement.Scope == capability.ScopeApplication {
					host, hostErr = devaccess.ApplicationHost(resolved.Target.Name, resolved.Manifest.Name, "prometheus")
				}
			}
			if hostErr == nil && strings.TrimSpace(host) != "" {
				managementUI[i].URL = devaccess.CanonicalURL(host)
			}
		}
	}

	var runtimeArtifact *runtimeArtifactObservation
	var runtimeDocsURL string
	if application.RequiresRuntimeBroker(resolved.Manifest) && result.State != "not_applied" {
		if files, filesErr := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest); filesErr == nil {
			if brokerFiles, brokerErr := runtimebroker.Existing(files); brokerErr == nil {
				runtimeDocsURL = strings.TrimSpace(brokerFiles.DocsURL)
				runtimeArtifact = &runtimeArtifactObservation{
					Reference:       strings.TrimSpace(brokerFiles.Image),
					ExpectedVersion: strings.TrimSpace(version),
				}
				if compose, composeErr := bhruntime.DetectCompose(ctx); composeErr == nil {
					identity, identityErr := compose.ProjectServiceImageIdentity(ctx, runtimebroker.ProjectNameForRuntime(resolved.Manifest, files), runtimebroker.ServiceName)
					if identityErr != nil {
						runtimeArtifact.Detail = identityErr.Error()
					} else {
						runtimeArtifact.Reference = identity.Reference
						runtimeArtifact.ImageID = identity.ImageID
						runtimeArtifact.Digest = identity.Digest
					}
				} else {
					runtimeArtifact.Detail = composeErr.Error()
				}
			}
		}
	}
	return applicationStatusResult{
		StatusResult:    result,
		OperatorAuth:    collectOperatorAuthObservation(ctx, resolved.Target.Name, resolved.Manifest.Environment),
		TLS:             tlsObservation,
		ServiceTLS:      serviceTLS,
		RuntimeArtifact: runtimeArtifact,
		RuntimeDocsURL:  runtimeDocsURL,
		ManagementUI:    managementUI,
		tlsStatus:       tlsStatus,
		tlsErr:          tlsErr,
		serviceTLSErr:   serviceTLSErr,
	}, nil
}
