package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/machine"
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
	compose           bhruntime.Compose
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
		{Name: "managed runtime definition", Run: func(context.Context) error {
			if c.runtimeErr != nil {
				return c.runtimeErr
			}
			return application.CheckManagedRuntimeDefinition(c.files, m)
		}},
		{Name: "runtime orchestration", Run: func(ctx context.Context) error {
			var err error
			c.compose, err = detectComposeForApplication(ctx, c.resolved, bhruntime.CapabilityWorkloadLifecycle, bhruntime.CapabilityResourceOwnership)
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
	if m.Services.SQL {
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
	if m.Services.Cache {
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
