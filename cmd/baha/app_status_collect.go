package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
)

type applicationStatusCollection struct {
	resolved       resolvedApplication
	manifest       application.Manifest
	files          application.RuntimeFiles
	compose        bhruntime.Compose
	services       []string
	result         application.StatusResult
	workloadStatus repositoryWorkloadStatus
	workloadErr    error
}

func newApplicationStatusCollection(ctx context.Context, store application.Store, args []string) (*applicationStatusCollection, bool, error) {
	resolved, err := resolveApplication(ctx, store, args, "status")
	if err != nil {
		return &applicationStatusCollection{}, false, err
	}
	m := resolved.Manifest
	if err := application.CheckSupportedRuntimeServices(m); err != nil {
		return &applicationStatusCollection{}, false, err
	}

	files, err := application.ExistingRuntimeFiles(resolved.Store, m)
	if errors.Is(err, application.ErrRuntimeNotApplied) {
		result := application.StatusResult{
			ContractVersion: "v1",
			Target:          resolved.Target.Name,
			Application:     m.Name,
			Environment:     m.Environment,
			Project:         application.RuntimeComposeProjectNameForStore(resolved.Store, m),
			State:           "not_applied",
			Ready:           false,
			Checks:          []application.StatusCheck{},
		}
		if resolved.IncompleteDeployment {
			result.State = "incomplete"
			result.AddCheck("deployment-state", false, "deployment convergence is incomplete; no final deployment record exists")
		}
		if resolved.FromRepository {
			result.Manifest = resolved.ManifestPath
		}
		return &applicationStatusCollection{resolved: resolved, manifest: m, result: result}, true, nil
	}
	if err != nil {
		return &applicationStatusCollection{}, false, err
	}

	compose, err := detectComposeForTarget(ctx, resolved.Target)
	if err != nil {
		return &applicationStatusCollection{}, false, err
	}
	services, err := compose.RunningServicesProject(ctx, files.Project, files.Compose, files.Env)
	if err != nil {
		return &applicationStatusCollection{}, false, err
	}

	result := application.StatusResult{
		ContractVersion: "v1",
		Target:          resolved.Target.Name,
		Application:     m.Name,
		Environment:     m.Environment,
		Project:         files.Project,
		State:           "running",
		Ready:           !resolved.IncompleteDeployment,
		Checks:          []application.StatusCheck{},
	}
	if resolved.IncompleteDeployment {
		result.AddCheck("deployment-state", false, "deployment convergence is incomplete; protected application state was recovered without a final deployment record")
	}
	if resolved.FromRepository {
		result.Manifest = resolved.ManifestPath
	}

	return &applicationStatusCollection{
		resolved: resolved,
		manifest: m,
		files:    files,
		compose:  compose,
		services: services,
		result:   result,
	}, false, nil
}

func (c *applicationStatusCollection) componentsStopped(ctx context.Context) bool {
	c.workloadStatus, c.workloadErr = inspectRepositoryWorkloadStatus(ctx, c.compose, c.resolved, c.files)

	workloadRunning := make([]string, 0, len(c.workloadStatus.Services))
	for _, service := range c.workloadStatus.Services {
		if service.State == "running" {
			workloadRunning = append(workloadRunning, service.Service)
		}
	}

	brokerRunning := false
	if application.RequiresRuntimeBroker(c.manifest) {
		if brokerFiles, brokerErr := runtimebroker.Existing(c.files); brokerErr == nil {
			if running, runErr := c.compose.RunningServicesProject(ctx, runtimebroker.ProjectNameForRuntime(c.manifest, c.files), brokerFiles.Compose, c.files.Env); runErr == nil {
				brokerRunning = len(running) > 0
			}
		}
	}

	exposureRunning := managedExposureRunning(ctx, c.compose, c.manifest, c.files)
	return applicationComponentsStopped(c.services, workloadRunning, c.workloadStatus.Found, brokerRunning, exposureRunning) && !application.HasObjectStorage(c.manifest)
}

func (c *applicationStatusCollection) collectManagedServiceChecks(ctx context.Context) {
	c.collectObjectStorageCheck(ctx)
	c.collectTelemetryCheck(ctx)
	c.collectIdentityCheck(ctx)
	c.collectServiceBindingCheck()
	c.collectSQLCheck(ctx)
	c.collectCacheCheck(ctx)
	c.collectManagementUICheck(ctx)
	c.collectSecretsAndBrokerChecks(ctx)
}

func (c *applicationStatusCollection) collectServiceBindingCheck() {
	if !application.HasManagedRuntimeServices(c.manifest) {
		return
	}
	if err := application.VerifyWorkloadServiceBindings(c.manifest, c.files); err != nil {
		c.result.AddCheck("service-bindings", false, err.Error())
		return
	}
	c.result.AddCheck("service-bindings", true, "standard workload service bindings projected and verified")
}

func (c *applicationStatusCollection) collectObjectStorageCheck(ctx context.Context) {
	if !application.HasObjectStorage(c.manifest) {
		return
	}
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	err := objectstorage.VerifyApplicationBucketsAt(checkCtx, c.compose, c.manifest, c.files, c.resolved.TargetStateRoot, c.resolved.Target.Name)
	cancel()
	if err != nil {
		c.result.AddCheck("object-storage", false, err.Error())
		return
	}
	c.result.AddCheck("object-storage", true, fmt.Sprintf("%d bucket(s) passed authenticated S3 Put/Get", len(application.ObjectStorageBucketNames(c.manifest))))
}

func (c *applicationStatusCollection) collectIdentityCheck(ctx context.Context) {
	if !application.HasIdentity(c.manifest) {
		return
	}
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := verifyExistingManagedIdentity(checkCtx, c.compose, c.resolved, nil); err != nil {
		c.result.AddCheck("identity/oidc", false, err.Error())
		return
	}
	bindings, err := application.CapabilityBindings(c.manifest)
	if err != nil {
		c.result.AddCheck("identity/oidc", false, err.Error())
		return
	}
	provider := "unknown"
	for _, binding := range bindings {
		if binding.Resource.Kind == capability.Identity {
			provider = string(binding.Resource.Provider)
			break
		}
	}
	c.result.AddCheck("identity/oidc", true, "OIDC discovery, binding and provider state verified via "+provider)
}

func (c *applicationStatusCollection) collectTelemetryCheck(ctx context.Context) {
	if !application.HasOTLPTelemetry(c.manifest) {
		return
	}
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	err := telemetry.VerifyApplicationAt(checkCtx, c.manifest, c.files, c.resolved.TargetStateRoot, c.resolved.Target.Name)
	cancel()
	if err != nil {
		c.result.AddCheck("telemetry/otlp", false, err.Error())
		return
	}
	c.result.AddCheck("telemetry/otlp", true, "real OTLP HTTP/protobuf export accepted")
}

func (c *applicationStatusCollection) collectSQLCheck(ctx context.Context) {
	if !c.manifest.Services.SQL {
		return
	}
	if !containsString(c.services, "postgres") {
		c.result.AddCheck("postgres", false, "not running")
		return
	}
	checkCtx, cancel := context.WithTimeout(ctx, applicationPostgresStatusTimeout)
	err := application.VerifyPostgresRuntime(checkCtx, c.compose, c.manifest, c.files)
	cancel()
	if err != nil {
		c.result.AddCheck("postgres", false, "one or more instances failed readiness")
		return
	}
	c.result.AddCheck("postgres", true, fmt.Sprintf("%d instance(s) running and authenticated SELECT 1 succeeded", len(application.SQLInstanceNames(c.manifest))))
}

func (c *applicationStatusCollection) collectCacheCheck(ctx context.Context) {
	if !c.manifest.Services.Cache {
		return
	}
	if !containsString(c.services, "valkey") {
		c.result.AddCheck("valkey", false, "not running")
		return
	}
	checkCtx, cancel := context.WithTimeout(ctx, applicationValkeyStatusTimeout)
	err := application.VerifyValkeyRuntime(checkCtx, c.compose, c.manifest, c.files)
	cancel()
	if err != nil {
		c.result.AddCheck("valkey", false, "one or more instances failed authenticated PING")
		return
	}
	c.result.AddCheck("valkey", true, fmt.Sprintf("%d instance(s) running and authenticated PING returned PONG", len(application.CacheInstanceNames(c.manifest))))
}

func (c *applicationStatusCollection) collectManagementUICheck(ctx context.Context) {
	if !c.manifest.Services.SQLManagementUI &&
		!c.manifest.Services.CacheManagementUI &&
		!c.manifest.Services.ObjectStorageManagementUI &&
		!c.manifest.Services.SecretsManagementUI &&
		!c.manifest.Services.IdentityManagementUI &&
		!c.manifest.Services.ObservabilityManagementUI {
		return
	}

	selected := 0
	ready := 0
	record := func(name string, err error, detail string) {
		selected++
		if err != nil {
			c.result.AddCheck("management-ui/"+name, false, err.Error())
			return
		}
		ready++
		c.result.AddCheck("management-ui/"+name, true, detail)
	}

	for _, result := range application.VerifyApplicationManagementUIChecks(ctx, c.manifest, c.files) {
		detail := result.Name + " reachable over TLS"
		if devaccess.Enabled(c.manifest.Environment) {
			service := result.Name
			if result.Name == "redis-commander" {
				service = "cache"
			} else if result.Name == "pgadmin" {
				service = "pgadmin"
			}
			if host, hostErr := devaccess.ApplicationHost(c.resolved.Target.Name, c.manifest.Name, service); hostErr == nil {
				detail = devaccess.CanonicalURL(host)
			}
		}
		record(result.Name, result.Err, detail)
	}

	if c.manifest.Services.ObjectStorageManagementUI {
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := objectstorage.VerifyManagementUIAt(checkCtx, c.resolved.TargetStateRoot, c.resolved.Target.Name)
		cancel()
		detail := "object storage management UI reachable over TLS"
		if devaccess.Enabled(c.manifest.Environment) {
			if placement, placementErr := application.ResolveProviderPlacement(c.manifest, capability.ProviderSeaweedFS); placementErr == nil && placement.Scope != capability.ScopeExternal {
				var host string
				var hostErr error
				if placement.Scope == capability.ScopeShared {
					host, hostErr = devaccess.SharedHost(c.resolved.Target.Name, "storage")
				} else {
					host, hostErr = devaccess.ApplicationHost(c.resolved.Target.Name, c.manifest.Name, "storage")
				}
				if hostErr == nil {
					detail = devaccess.CanonicalURL(host)
				}
			}
		}
		record("object-storage", err, detail)
	}

	if c.manifest.Services.SecretsManagementUI {
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		platformFiles, err := existingTargetRuntimeFiles(checkCtx)
		if err == nil {
			err = verifyOpenBaoManagementUI(checkCtx, platformFiles)
		}
		cancel()
		detail := "OpenBao management UI reachable over TLS"
		if devaccess.Enabled(c.manifest.Environment) {
			if host, hostErr := devaccess.SharedHost(c.resolved.Target.Name, "openbao"); hostErr == nil {
				detail = devaccess.CanonicalURL(host)
			}
		}
		record("openbao", err, detail)
	}

	if c.manifest.Services.IdentityManagementUI {
		files, err := identityprovider.ExistingKeycloakFilesAt(c.manifest, c.resolved.TargetStateRoot, c.resolved.Target.Name)
		if err != nil {
			record("identity-login", err, "")
			record("identity-admin", err, "")
		} else {
			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			client, checkErr := serviceaccess.NewHTTPClient(files.PublicAccess.Material, false)
			if checkErr == nil {
				checkErr = serviceaccess.WaitHTTPS(checkCtx, client, files.PublicURL, "/")
			}
			cancel()
			loginDetail := "Keycloak user-facing identity UI reachable over TLS"
			if devaccess.Enabled(c.manifest.Environment) {
				if host, hostErr := devaccess.ApplicationHost(c.resolved.Target.Name, c.manifest.Name, "identity"); hostErr == nil {
					loginDetail = devaccess.CanonicalURL(host)
				}
			}
			record("identity-login", checkErr, loginDetail)

			checkCtx, cancel = context.WithTimeout(ctx, 5*time.Second)
			client, checkErr = serviceaccess.NewHTTPClient(files.AdminAccess.Material, false)
			if checkErr == nil {
				checkErr = serviceaccess.WaitHTTPS(checkCtx, client, files.AdminURL, "/")
			}
			cancel()
			adminDetail := "Keycloak administration UI reachable over TLS"
			if devaccess.Enabled(c.manifest.Environment) {
				if host, hostErr := devaccess.ApplicationHost(c.resolved.Target.Name, c.manifest.Name, "identity-admin"); hostErr == nil {
					adminDetail = devaccess.CanonicalURL(host)
				}
			}
			record("identity-admin", checkErr, adminDetail)
		}
	}

	if c.manifest.Services.ObservabilityManagementUI {
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := metricsprovider.VerifyManagementUIAt(checkCtx, c.resolved.TargetStateRoot, c.resolved.Target.Name, c.manifest)
		cancel()
		detail := "Prometheus management UI reachable over TLS"
		if devaccess.Enabled(c.manifest.Environment) {
			if placement, placementErr := metricsprovider.PlacementForAt(c.resolved.TargetStateRoot, c.resolved.Target.Name, c.manifest); placementErr == nil {
				var host string
				var hostErr error
				if placement.Scope == capability.ScopeShared {
					host, hostErr = devaccess.SharedHost(c.resolved.Target.Name, "prometheus")
				} else {
					host, hostErr = devaccess.ApplicationHost(c.resolved.Target.Name, c.manifest.Name, "prometheus")
				}
				if hostErr == nil {
					detail = devaccess.CanonicalURL(host)
				}
			}
		}
		record("prometheus", err, detail)
	}

	c.result.AddCheck("management-ui", ready == selected, fmt.Sprintf("%d/%d selected management UI surface(s) reachable", ready, selected))
}

func (c *applicationStatusCollection) collectSecretsAndBrokerChecks(ctx context.Context) {
	if !c.manifest.Services.Secrets {
		return
	}

	platformFiles, platformErr := existingTargetRuntimeFiles(ctx)
	if platformErr != nil {
		c.result.AddCheck("secrets", false, "BaseHarbor OpenBao runtime is not materialized")
	} else {
		scopeCtx, scopeCancel := context.WithTimeout(ctx, applicationOpenBaoStatusTimeout)
		identity := openbao.ApplicationIdentity{Name: c.manifest.Name, Environment: c.manifest.Environment}
		err := openbao.InspectApplicationScope(scopeCtx, c.compose, platformFiles, identity, openbao.ApplicationCredentialsPath(c.files.Dir))
		scopeCancel()
		if err != nil {
			c.result.AddCheck("secrets", false, "isolated OpenBao application scope is not ready")
		} else {
			c.result.AddCheck("secrets", true, "isolated OpenBao AppRole authentication succeeded")
			c.collectRequiredSecretChecks(ctx, platformFiles)
		}
	}

	brokerCtx, brokerCancel := context.WithTimeout(ctx, applicationBrokerStatusTimeout)
	brokerErr := verifyRuntimeBrokerRunning(brokerCtx, c.compose, c.manifest, c.files)
	brokerCancel()
	if brokerErr != nil {
		c.result.AddCheck("runtime-broker", false, brokerErr.Error())
		return
	}
	c.result.AddCheck("runtime-broker", true, "mTLS identity and app-scoped OpenBao readiness succeeded")
}

func (c *applicationStatusCollection) collectRequiredSecretChecks(ctx context.Context, platformFiles bhruntime.Files) {
	if len(application.RequiredSecretNames(c.manifest)) == 0 {
		return
	}
	secretCtx, secretCancel := context.WithTimeout(ctx, applicationRequiredSecretTimeout)
	statuses, statusErr := inspectRequiredApplicationSecrets(secretCtx, c.compose, platformFiles, c.manifest, c.files)
	secretCancel()
	if statusErr != nil {
		c.result.AddCheck("required-secrets", false, "readiness inspection failed")
		return
	}
	for _, status := range statuses {
		ok := status.Present && status.Usable
		detail := "missing or unusable"
		if ok {
			detail = "present and usable"
		}
		c.result.AddCheck("required-secret/"+status.Name, ok, detail)
	}
}

func (c *applicationStatusCollection) collectWorkloadChecks() {
	if c.workloadStatus.Found {
		for _, service := range c.workloadStatus.Services {
			c.result.AddCheck("workload/"+service.Service, service.Ready, formatWorkloadServiceStatus(service))
		}
		if c.workloadErr != nil {
			c.result.AddCheck("workload", false, c.workloadErr.Error())
		} else {
			c.result.AddCheck("workload", c.workloadStatus.Ready(), fmt.Sprintf("%d/%d selected Compose service(s) ready", c.workloadStatus.ReadyCount(), len(c.workloadStatus.Services)))
		}
		return
	}
	if c.workloadErr != nil {
		c.result.AddCheck("workload", false, "repository Compose integration could not be resolved: "+c.workloadErr.Error())
	}
}

func (c *applicationStatusCollection) collectLogsCheck(ctx context.Context) {
	if !application.HasLogsCollection(c.manifest) {
		return
	}
	policy, policyErr := application.LogsPolicy(c.manifest)
	if policyErr != nil {
		c.result.AddCheck("logs", false, policyErr.Error())
		return
	}
	if !policy.Enabled || !policy.Collect[application.LogsSourceApplication] || !c.workloadStatus.Found {
		return
	}

	logServices := make([]string, 0, len(c.workloadStatus.Services))
	for _, service := range c.workloadStatus.Services {
		logServices = append(logServices, service.Service)
	}
	checkCtx, cancel := context.WithTimeout(ctx, applicationLogsStatusTimeout)
	err := logsprovider.VerifyApplicationAt(checkCtx, c.manifest, logServices, c.resolved.TargetStateRoot, c.resolved.Target.Name)
	cancel()
	if err != nil {
		c.result.AddCheck("logs", false, err.Error())
		return
	}
	c.result.AddCheck("logs", true, fmt.Sprintf("%d workload log stream(s) queryable", len(logServices)))
}

func (c *applicationStatusCollection) collectExposureCheck(ctx context.Context) {
	if len(c.manifest.Exposures) == 0 {
		return
	}
	_, exposureErr := inspectManagedExposure(ctx, c.compose, c.manifest, c.files)
	if exposureErr != nil {
		c.result.AddCheck("managed-exposure", false, exposureErr.Error())
		return
	}
	if devaccess.Enabled(c.manifest.Environment) {
		for _, route := range c.manifest.Exposures {
			label := route.Name
			if len(c.manifest.Exposures) == 1 {
				label = "api"
			}
			host, err := devaccess.ApplicationHost(c.resolved.Target.Name, c.manifest.Name, label)
			if err != nil {
				c.result.AddCheck("managed-exposure/"+route.Name, false, err.Error())
				continue
			}
			c.result.AddCheck("managed-exposure/"+route.Name, true, devaccess.CanonicalURL(host))
		}
		return
	}
	c.result.AddCheck("managed-exposure", true, "configured exposure endpoints are ready")
}
