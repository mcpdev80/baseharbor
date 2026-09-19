package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func appUpCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "up",
		Summary: "Start an existing application runtime and verify readiness",
		Usage:   "baha app up [NAME]",
		Long:    "Starts a previously materialized BaseHarbor application runtime using its existing runtime definition, credentials and persistent data; required application secrets are verified before workload start and missing values fail closed. Repository workloads are started after their BaseHarbor backend and per-application secret broker are ready. Workload-only applications skip the empty managed-runtime start and resume their repository Compose workload directly. Without NAME it resolves the nearest repository baseharbor.yaml.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			resolved, err := resolveApplication(store, args, "up")
			if err != nil {
				return err
			}
			m := resolved.Manifest
			files, err := application.ExistingRuntimeFiles(resolved.Store, m)
			if err != nil {
				return err
			}

			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			var compose bhruntime.Compose
			var before []bhruntime.ProjectResource
			var platformFiles bhruntime.Files
			var managedExposure *managedExposureExecution
			var managedObjectStorage *managedObjectStorageExecution
			var managedTelemetry *managedTelemetryExecution
			checks := []preflight.Check{
				{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
				{Name: "supported desired services", Run: func(context.Context) error { return application.CheckSupportedRuntimeServices(m) }},
				{Name: "manifest permissions", Run: func(context.Context) error {
					return checkManifestPermissions(resolved.ManifestPath, resolved.FromRepository)
				}},
				{Name: "application workload", Run: func(context.Context) error { return preflightRepositoryWorkload(resolved) }},
				{Name: "runtime permissions", Run: func(context.Context) error { return application.CheckRuntimePermissions(files) }},
				{Name: "managed runtime definition", Run: func(context.Context) error { return application.CheckManagedRuntimeDefinition(files, m) }},
				{Name: "runtime provider capabilities", Run: func(ctx context.Context) error {
					required := []bhruntime.RuntimeCapability{
						bhruntime.CapabilityWorkloadLifecycle,
						bhruntime.CapabilityResourceOwnership,
					}
					if m.Services.Secrets || application.HasObjectStorage(m) {
						required = append(required, bhruntime.CapabilityServiceExec)
					}
					var err error
					compose, err = detectComposeForApplication(ctx, resolved, required...)
					return err
				}},
				{Name: "provider registry", Run: func(context.Context) error {
					return application.CheckReferenceProviderRegistry(m)
				}},
				{Name: "managed object storage provider", Run: func(ctx context.Context) error {
					var err error
					managedObjectStorage, err = prepareManagedObjectStorage(ctx, compose, resolved)
					return err
				}},
				{Name: "managed telemetry provider", Run: func(ctx context.Context) error {
					var err error
					managedTelemetry, err = prepareManagedTelemetry(ctx, compose, resolved)
					return err
				}},
				{Name: "managed exposure provider", Run: func(ctx context.Context) error {
					var err error
					managedExposure, err = prepareManagedExposure(ctx, compose, resolved)
					return err
				}},
				{Name: "compose configuration", Run: func(ctx context.Context) error {
					if !application.HasManagedRuntimeServices(m) {
						return nil
					}
					return compose.ConfigProject(ctx, application.RuntimeProjectName(m), files.Compose, files.Env)
				}},
				{Name: "runtime ownership", Run: func(ctx context.Context) error {
					var err error
					before, err = application.InspectOwnedRuntimeResources(ctx, compose, m)
					return err
				}},
				{Name: "persistent data volumes", Run: func(context.Context) error {
					for _, volume := range application.ExpectedPersistentRuntimeResources(m) {
						if !application.ResourceNamedExists(before, volume) {
							return fmt.Errorf("managed %s is missing; refusing to recreate persistent state during app up", volume.Name)
						}
					}
					return nil
				}},
			}
			if m.Services.Secrets {
				checks = append(checks,
					preflight.Check{Name: "OpenBao control-plane runtime", Run: func(context.Context) error {
						var err error
						platformFiles, err = bhruntime.ExistingFiles("")
						return err
					}},
					preflight.Check{Name: "OpenBao application scope", Run: func(ctx context.Context) error {
						if platformFiles.Compose == "" {
							return errors.New("BaseHarbor OpenBao runtime is not materialized")
						}
						identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
						return openbao.InspectApplicationScope(ctx, compose, platformFiles, identity, openbao.ApplicationCredentialsPath(files.Dir))
					}},
					preflight.Check{Name: "required application secrets", Run: func(ctx context.Context) error {
						return checkRequiredApplicationSecrets(ctx, compose, platformFiles, m, files)
					}},
				)
			}
			results, ok := preflight.Run(checkCtx, checks)
			preflight.Format(out, results)
			if !ok {
				return errors.New("application up preflight failed")
			}

			if application.HasManagedRuntimeServices(m) {
				project := application.RuntimeProjectName(m)
				if err := compose.UpProject(ctx, project, files.Compose, files.Env); err != nil {
					return err
				}
			}
			if err := convergeManagedObjectStorage(ctx, out, managedObjectStorage); err != nil {
				return fmt.Errorf("converge managed object storage: %w", err)
			}

			verifyCtx, verifyCancel := context.WithTimeout(ctx, 60*time.Second)
			defer verifyCancel()
			var verifyErr error
			for verifyCtx.Err() == nil {
				verifyErr = verifyDesiredRuntimeServices(verifyCtx, compose, m, files)
				if verifyErr == nil && m.Services.Secrets {
					identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
					verifyErr = openbao.CheckApplicationScope(verifyCtx, compose, platformFiles, identity, openbao.ApplicationCredentialsPath(files.Dir))
					if verifyErr == nil {
						verifyErr = checkRequiredApplicationSecrets(verifyCtx, compose, platformFiles, m, files)
					}
				}
				if verifyErr == nil {
					break
				}
				select {
				case <-verifyCtx.Done():
				case <-time.After(time.Second):
				}
			}
			if verifyErr != nil {
				return fmt.Errorf("application verification failed: %w", verifyErr)
			}
			if m.Services.Secrets {
				if err := ensureAndStartRuntimeBroker(ctx, compose, platformFiles, m, files); err != nil {
					return err
				}
			}
			printRuntimeReady(out, m)
			if err := convergeManagedTelemetry(ctx, out, managedTelemetry); err != nil {
				return fmt.Errorf("converge managed telemetry: %w", err)
			}
			if _, err := applyRepositoryWorkload(ctx, out, compose, resolved, files); err != nil {
				return err
			}
			if err := convergeManagedExposure(ctx, out, managedExposure); err != nil {
				return fmt.Errorf("converge managed HTTP exposure: %w", err)
			}
			if err := application.ReconcileReferenceProviderRegistry(m); err != nil {
				return fmt.Errorf("record provider registry after successful restart: %w", err)
			}
			fmt.Fprintf(out, "Application %s is running and ready.\n", m.Name)
			return nil
		},
	}
}
