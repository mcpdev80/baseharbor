package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/machine"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
)

type applicationDoctorCollector struct {
	resolved          resolvedApplication
	manifest          application.Manifest
	result            applicationDoctorResult
	files             application.RuntimeFiles
	runtimeErr        error
	serviceTLS        []application.BackendTLSLifecycleObservation
	serviceTLSErr     error
	compose           bhruntime.RuntimeProvider
	running           []string
	platformFiles     bhruntime.Files
	requiredStatuses  []openbao.RequiredSecretStatus
	workloadStatus    repositoryWorkloadStatus
	workloadStatusErr error
	workloadSecurity  application.WorkloadSecurityReport
	results           []preflight.Result
	ok                bool
	tlsStatus         *applicationTLSStatus
	tlsObservation    *applicationTLSObservation
	tlsErr            error
}

func newApplicationDoctorCollector(ctx context.Context, store application.Store, args []string) (*applicationDoctorCollector, bool, error) {
	resolved, err := resolveApplication(ctx, store, args, "doctor")
	if err != nil {
		return &applicationDoctorCollector{}, false, err
	}
	m := resolved.Manifest
	result := applicationDoctorResult{
		ContractVersion: machine.ContractVersion,
		Target:          resolved.Target.Name,
		Application:     m.Name,
		Environment:     m.Environment,
		State:           "ready",
		Healthy:         true,
		Checks:          []preflight.Result{},
		OperatorAuth:    collectOperatorAuthObservation(ctx, resolved.Target.Name, m.Environment),
		manifest:        m,
	}

	files, runtimeErr := application.ExistingRuntimeFiles(resolved.Store, m)
	if errors.Is(runtimeErr, application.ErrRuntimeNotApplied) {
		result.State = "not_applied"
		result.Healthy = false
		return &applicationDoctorCollector{resolved: resolved, manifest: m, result: result, runtimeErr: runtimeErr}, true, nil
	}

	collector := &applicationDoctorCollector{
		resolved:   resolved,
		manifest:   m,
		result:     result,
		files:      files,
		runtimeErr: runtimeErr,
	}
	if runtimeErr == nil {
		collector.serviceTLS, collector.serviceTLSErr = application.InspectBackendTLSLifecycle(files, m)
		if collector.serviceTLSErr == nil && application.HasSharedBackends(m) {
			sharedTLS, err := application.InspectSharedBackendTLSLifecycleAt(resolved.TargetStateRoot, resolved.Target.Name, m)
			if err != nil {
				collector.serviceTLSErr = err
			} else {
				collector.serviceTLS = append(collector.serviceTLS, sharedTLS...)
			}
		}
	}
	return collector, false, nil
}

func (c *applicationDoctorCollector) runChecks(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	checks := c.baseChecks()
	checks = c.appendObservabilityChecks(checks)
	checks = c.appendBackendChecks(checks)
	checks = c.appendSecretChecks(checks)
	c.results, c.ok = preflight.Run(checkCtx, checks)
}

func (c *applicationDoctorCollector) baseChecks() []preflight.Check {
	m := c.manifest
	return []preflight.Check{
		{Name: "operator authentication", Run: func(context.Context) error {
			if c.result.OperatorAuth.Status == "DEGRADED" || c.result.OperatorAuth.Status == "NOT_CONFIGURED" {
				return fmt.Errorf("%s", c.result.OperatorAuth.Detail)
			}
			return nil
		}},
		{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
		{Name: "supported desired services", Run: func(context.Context) error { return application.CheckSupportedRuntimeServices(m) }},
		{Name: "manifest permissions", Run: func(context.Context) error {
			return checkManifestPermissions(c.resolved.ManifestPath, c.resolved.FromRepository)
		}},
		{Name: "workload discovery", Run: func(context.Context) error { return preflightRepositoryWorkload(c.resolved) }},
		{Name: "runtime state", Run: func(context.Context) error { return c.runtimeErr }},
		{Name: "runtime permissions", Run: func(context.Context) error {
			if c.runtimeErr != nil {
				return c.runtimeErr
			}
			return application.CheckRuntimePermissions(c.files)
		}},
		{Name: "service TLS lifecycle", Run: func(context.Context) error {
			if c.runtimeErr != nil {
				return c.runtimeErr
			}
			if c.serviceTLSErr != nil {
				return c.serviceTLSErr
			}
			if !serviceTLSLifecycleHealthy(c.serviceTLS) {
				return errors.New("one or more service certificates require immediate rotation")
			}
			return nil
		}},
		{Name: "managed identity", Run: func(ctx context.Context) error {
			if !application.HasIdentity(m) {
				return nil
			}
			if c.runtimeErr != nil {
				return c.runtimeErr
			}
			return verifyExistingManagedIdentity(ctx, c.compose, c.resolved, nil)
		}},
		{Name: "canonical development URLs", Run: func(ctx context.Context) error {
			if !requiresDevelopmentGateway(m) {
				return nil
			}
			hosts, err := applicationCanonicalRouteHosts(c.resolved.Target.Name, m)
			if err != nil {
				return err
			}
			verifyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			return devgateway.VerifyHosts(verifyCtx, c.resolved.Target.Name, hosts)
		}},
		{Name: "managed runtime definition", Run: func(context.Context) error {
			if c.runtimeErr != nil {
				return c.runtimeErr
			}
			return application.CheckManagedRuntimeDefinition(c.files, m)
		}},
		{Name: "runtime orchestration", Run: func(ctx context.Context) error {
			var err error
			c.compose, err = detectRuntimeForApplication(ctx, c.resolved, bhruntime.CapabilityWorkloadLifecycle, bhruntime.CapabilityResourceOwnership)
			return err
		}},
		{Name: "workload security", Run: func(ctx context.Context) error {
			var err error
			c.workloadSecurity, err = preflightRepositoryWorkloadSecurity(ctx, c.compose, c.resolved)
			return err
		}},
		{Name: "runtime configuration", Run: func(ctx context.Context) error {
			if c.runtimeErr != nil {
				return c.runtimeErr
			}
			return c.compose.ConfigProject(ctx, c.files.Project, c.files.Compose, c.files.Env)
		}},
		{Name: "running services", Run: func(ctx context.Context) error {
			if c.runtimeErr != nil {
				return c.runtimeErr
			}
			var err error
			c.running, err = c.compose.RunningServicesProject(ctx, c.files.Project, c.files.Compose, c.files.Env)
			return err
		}},
		{Name: "repository workload", Run: func(ctx context.Context) error {
			if c.runtimeErr != nil {
				return c.runtimeErr
			}
			c.workloadStatus, c.workloadStatusErr = inspectRepositoryWorkloadStatus(ctx, c.compose, c.resolved, c.files)
			if c.workloadStatusErr != nil {
				return c.workloadStatusErr
			}
			if c.workloadStatus.Found && !c.workloadStatus.Ready() {
				return fmt.Errorf("%d/%d selected workload services ready", c.workloadStatus.ReadyCount(), len(c.workloadStatus.Services))
			}
			return nil
		}},
	}
}

func (c *applicationDoctorCollector) appendObservabilityChecks(checks []preflight.Check) []preflight.Check {
	m := c.manifest
	if application.HasLogsCollection(m) {
		if policy, policyErr := application.LogsPolicy(m); policyErr != nil {
			checks = append(checks, preflight.Check{Name: "logs deployment policy", Run: func(context.Context) error { return policyErr }})
		} else if policy.Enabled && policy.Collect[application.LogsSourceApplication] {
			checks = append(checks, preflight.Check{Name: "Loki log ingestion", Run: func(ctx context.Context) error {
				if c.runtimeErr != nil {
					return c.runtimeErr
				}
				status, err := inspectRepositoryWorkloadStatus(ctx, c.compose, c.resolved, c.files)
				if err != nil {
					return err
				}
				if !status.Found {
					return nil
				}
				services := make([]string, 0, len(status.Services))
				for _, service := range status.Services {
					services = append(services, service.Service)
				}
				return logsprovider.VerifyApplicationAt(ctx, m, services, c.resolved.TargetStateRoot, c.resolved.Target.Name)
			}})
		}
	}
	if len(m.Exposures) > 0 {
		checks = append(checks, preflight.Check{Name: "managed HTTP exposure", Run: func(ctx context.Context) error {
			if c.runtimeErr != nil {
				return c.runtimeErr
			}
			_, err := inspectManagedExposure(ctx, c.compose, m, c.files)
			return err
		}})
	}
	if application.HasObjectStorage(m) {
		checks = append(checks, preflight.Check{Name: "object-storage S3 readiness", Run: func(ctx context.Context) error {
			if c.runtimeErr != nil {
				return c.runtimeErr
			}
			return objectstorage.VerifyApplicationBucketsAt(ctx, c.compose, m, c.files, c.resolved.TargetStateRoot, c.resolved.Target.Name)
		}})
	}
	if application.HasOTLPTelemetry(m) {
		checks = append(checks, preflight.Check{Name: "OTLP telemetry export", Run: func(ctx context.Context) error {
			if c.runtimeErr != nil {
				return c.runtimeErr
			}
			return telemetry.VerifyApplicationAt(ctx, m, c.files, c.resolved.TargetStateRoot, c.resolved.Target.Name)
		}})
	}
	return checks
}

func (c *applicationDoctorCollector) appendBackendChecks(checks []preflight.Check) []preflight.Check {
	m := c.manifest
	if application.HasManagedRuntimeServices(m) {
		checks = append(checks, preflight.Check{Name: "workload service bindings", Run: func(context.Context) error {
			return application.VerifyWorkloadServiceBindings(m, c.files)
		}})
	}
	if m.Services.SQL {
		if application.UsesSharedPostgreSQL(m) {
			checks = append(checks, preflight.Check{Name: "postgres shared isolation", Run: func(ctx context.Context) error {
				return application.VerifySharedPostgreSQL(ctx, c.compose, c.resolved.TargetStateRoot, c.resolved.Target.Name, m)
			}})
			if resources, err := application.SharedPostgresResourcesAt(c.resolved.TargetStateRoot, c.resolved.Target.Name, m); err == nil {
				for _, resource := range resources {
					resource := resource
					checks = append(checks, preflight.Check{
						Name: "postgres/" + resource.Instance + " ownership",
						Run: func(context.Context) error {
							if resource.ProviderScope != "shared" || resource.CredentialScope != "application" || resource.Owner != m.Name+"/"+m.Environment {
								return fmt.Errorf(
									"unexpected shared PostgreSQL ownership: scope=%s owner=%s credential_scope=%s",
									resource.ProviderScope, resource.Owner, resource.CredentialScope,
								)
							}
							return nil
						},
					})
				}
			} else {
				checks = append(checks, preflight.Check{Name: "postgres shared resource ownership", Run: func(context.Context) error { return err }})
			}
		} else {
			checks = append(checks,
				preflight.Check{Name: "postgres running", Run: func(context.Context) error {
					if !containsString(c.running, "postgres") {
						return errors.New("no postgres instance is running")
					}
					return nil
				}},
				preflight.Check{Name: "postgres readiness", Run: func(ctx context.Context) error {
					if !containsString(c.running, "postgres") {
						return errors.New("no postgres instance is running")
					}
					return application.VerifyPostgresRuntime(ctx, c.compose, m, c.files)
				}},
			)
		}
	}
	if m.Services.Cache {
		if application.UsesSharedValkey(m) {
			checks = append(checks, preflight.Check{Name: "valkey shared isolation", Run: func(ctx context.Context) error {
				return application.VerifySharedValkey(ctx, c.compose, c.resolved.TargetStateRoot, c.resolved.Target.Name, m)
			}})
		} else {
			checks = append(checks,
				preflight.Check{Name: "valkey running", Run: func(context.Context) error {
					if !containsString(c.running, "valkey") {
						return errors.New("no valkey instance is running")
					}
					return nil
				}},
				preflight.Check{Name: "valkey readiness", Run: func(ctx context.Context) error {
					if !containsString(c.running, "valkey") {
						return errors.New("no valkey instance is running")
					}
					return application.VerifyValkeyRuntime(ctx, c.compose, m, c.files)
				}},
			)
		}
	}
	if m.Services.SQLManagementUI || m.Services.CacheManagementUI || m.Services.ObjectStorageManagementUI || m.Services.SecretsManagementUI || m.Services.ObservabilityManagementUI {
		checks = append(checks, preflight.Check{Name: "management UI readiness", Run: func(ctx context.Context) error {
			if c.runtimeErr != nil {
				return c.runtimeErr
			}
			if m.Services.SQLManagementUI || m.Services.CacheManagementUI {
				if err := application.VerifyApplicationManagementUIs(ctx, m, c.files); err != nil {
					return err
				}
				for _, result := range application.VerifySharedManagementUIChecks(ctx, c.resolved.TargetStateRoot, c.resolved.Target.Name, m) {
					if result.Err != nil {
						return result.Err
					}
				}
			}
			if m.Services.ObjectStorageManagementUI {
				if err := objectstorage.VerifyManagementUIAt(ctx, c.resolved.TargetStateRoot, c.resolved.Target.Name); err != nil {
					return err
				}
			}
			if m.Services.SecretsManagementUI {
				platformFiles, err := existingTargetRuntimeFiles(ctx)
				if err != nil {
					return err
				}
				if err := verifyOpenBaoManagementUI(ctx, platformFiles); err != nil {
					return err
				}
			}
			if m.Services.ObservabilityManagementUI {
				if err := metricsprovider.VerifyManagementUIAt(ctx, c.resolved.TargetStateRoot, c.resolved.Target.Name, m); err != nil {
					return err
				}
			}
			return nil
		}})
	}
	return checks
}

func (c *applicationDoctorCollector) appendSecretChecks(checks []preflight.Check) []preflight.Check {
	m := c.manifest
	if !m.Services.Secrets {
		return checks
	}

	checks = append(checks,
		preflight.Check{Name: "OpenBao control-plane runtime", Run: func(ctx context.Context) error {
			var err error
			c.platformFiles, err = existingTargetRuntimeFiles(ctx)
			if err != nil {
				return err
			}
			state, err := openbao.Inspect(ctx, c.compose, c.platformFiles)
			if err != nil {
				return err
			}
			if !state.Initialized {
				return openbao.ErrNotInitialized
			}
			if state.Sealed {
				return openbao.ErrSealed
			}
			return nil
		}},
		preflight.Check{Name: "OpenBao application scope", Run: func(ctx context.Context) error {
			if c.runtimeErr != nil {
				return c.runtimeErr
			}
			if c.platformFiles.Compose == "" {
				return errors.New("BaseHarbor OpenBao runtime is not materialized")
			}
			identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
			return openbao.InspectApplicationScope(ctx, c.compose, c.platformFiles, identity, openbao.ApplicationCredentialsPath(c.files.Dir))
		}},
		preflight.Check{Name: "application runtime broker", Run: func(ctx context.Context) error {
			if c.runtimeErr != nil {
				return c.runtimeErr
			}
			return verifyRuntimeBrokerRunning(ctx, c.compose, m, c.files)
		}},
	)
	if len(application.RequiredSecretNames(m)) > 0 {
		checks = append(checks, preflight.Check{Name: "required application secrets", Run: func(checkCtx context.Context) error {
			if c.runtimeErr != nil {
				return c.runtimeErr
			}
			var err error
			c.requiredStatuses, err = inspectRequiredApplicationSecrets(checkCtx, c.compose, c.platformFiles, m, c.files)
			if err != nil {
				return err
			}
			return openbao.RequireApplicationSecrets(c.requiredStatuses)
		}})
	}
	return checks
}

func (c *applicationDoctorCollector) collectTLS() {
	if !c.resolved.FromRepository {
		return
	}
	c.tlsStatus, c.tlsObservation, c.tlsErr = collectApplicationTLSObservation(c.resolved)
	if c.tlsErr != nil || (c.tlsObservation != nil && !c.tlsObservation.Healthy) {
		c.ok = false
	}
}

func (c *applicationDoctorCollector) finalize() {
	workloads := make([]applicationDoctorWorkloadResult, 0, len(c.workloadStatus.Services))
	for _, service := range c.workloadStatus.Services {
		workloads = append(workloads, applicationDoctorWorkloadResult{
			Service: service.Service,
			Ready:   service.Ready,
			Detail:  formatWorkloadServiceStatus(service),
		})
	}
	secrets := make([]applicationDoctorSecretResult, 0, len(c.requiredStatuses))
	for _, status := range c.requiredStatuses {
		secrets = append(secrets, applicationDoctorSecretResult{
			Name:      status.Name,
			Present:   status.Present,
			Usable:    status.Usable,
			Generated: status.Generated,
		})
	}

	c.result.Healthy = c.ok
	if !c.ok {
		c.result.State = "degraded"
	}
	c.result.Checks = c.results
	c.result.Workload = workloads
	c.result.RequiredSecrets = secrets
	c.result.TLS = c.tlsObservation
	c.result.ServiceTLS = c.serviceTLS
	c.result.workloadStatus = c.workloadStatus
	c.result.requiredSecretStatuses = c.requiredStatuses
	c.result.workloadSecurity = c.workloadSecurity
	c.result.tlsStatus = c.tlsStatus
	c.result.tlsErr = c.tlsErr
	c.result.serviceTLSErr = c.serviceTLSErr
}
