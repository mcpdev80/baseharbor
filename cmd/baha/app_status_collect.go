package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
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
			Project:         application.RuntimeProjectNameForStore(resolved.Store, m),
			State:           "not_applied",
			Ready:           false,
			Checks:          []application.StatusCheck{},
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
		Ready:           true,
		Checks:          []application.StatusCheck{},
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
	c.collectSQLCheck(ctx)
	c.collectCacheCheck(ctx)
	c.collectSecretsAndBrokerChecks(ctx)
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
	c.result.AddCheck("managed-exposure", true, "configured exposure endpoints are ready")
}
