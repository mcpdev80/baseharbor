package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func appStatusCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "status",
		Summary: "Show application runtime and readiness status",
		Usage:   "baha app status [NAME]",
		Long:    "Reports materialized runtime state, running services and protocol-level readiness. Without NAME it resolves the nearest repository baseharbor.yaml.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			resolved, err := resolveApplication(store, args, "status")
			if err != nil {
				return err
			}
			m := resolved.Manifest
			if err := application.CheckSupportedRuntimeServices(m); err != nil {
				return err
			}
			files, err := application.ExistingRuntimeFiles(resolved.Store, m)
			if err != nil {
				return err
			}
			compose, err := bhruntime.DetectCompose(ctx)
			if err != nil {
				return err
			}
			project := application.RuntimeProjectName(m)
			services, err := compose.RunningServicesProject(ctx, project, files.Compose, files.Env)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Application %s (%s)\n", m.Name, m.Environment)
			if resolved.FromRepository {
				fmt.Fprintf(out, "Manifest: %s\n", resolved.ManifestPath)
			}
			fmt.Fprintf(out, "Project: %s\n", project)

			ready := true
			if m.Services.Postgres {
				if !containsString(services, "postgres") {
					fmt.Fprintln(out, "[FAIL] postgres          not running")
					ready = false
				} else {
					checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					err := application.VerifyPostgresRuntime(checkCtx, compose, m, files)
					cancel()
					if err != nil {
						fmt.Fprintln(out, "[FAIL] postgres          one or more instances failed readiness")
						ready = false
					} else {
						fmt.Fprintf(out, "[OK] postgres          %d instance(s) running and authenticated SELECT 1 succeeded\n", len(application.PostgresInstanceNames(m)))
					}
				}
			}
			if m.Services.Redis {
				if !containsString(services, "valkey") {
					fmt.Fprintln(out, "[FAIL] valkey            not running")
					ready = false
				} else {
					checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					err := application.VerifyValkeyRuntime(checkCtx, compose, m, files)
					cancel()
					if err != nil {
						fmt.Fprintln(out, "[FAIL] valkey            one or more instances failed authenticated PING")
						ready = false
					} else {
						fmt.Fprintf(out, "[OK] valkey            %d instance(s) running and authenticated PING returned PONG\n", len(application.RedisInstanceNames(m)))
					}
				}
			}
			if m.Services.Secrets {
				platformFiles, platformErr := bhruntime.ExistingFiles("")
				if platformErr != nil {
					fmt.Fprintln(out, "[FAIL] secrets           BaseHarbor OpenBao runtime is not materialized")
					ready = false
				} else {
					checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
					identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
					err := openbao.InspectApplicationScope(checkCtx, compose, platformFiles, identity, openbao.ApplicationCredentialsPath(files.Dir))
					if err != nil {
						fmt.Fprintln(out, "[FAIL] secrets           isolated OpenBao application scope is not ready")
						ready = false
					} else {
						fmt.Fprintln(out, "[OK] secrets           isolated OpenBao AppRole authentication succeeded")
						if len(application.RequiredSecretNames(m)) > 0 {
							statuses, statusErr := inspectRequiredApplicationSecrets(checkCtx, compose, platformFiles, m, files)
							if statusErr != nil {
								fmt.Fprintln(out, "[FAIL] required-secrets  readiness inspection failed")
								ready = false
							} else {
								printRequiredSecretStatus(out, statuses)
								if err := openbao.RequireApplicationSecrets(statuses); err != nil {
									fmt.Fprintln(out, "[FAIL] required-secrets  one or more required secrets are missing or unusable")
									ready = false
								} else {
									fmt.Fprintln(out, "[OK] required-secrets  all required secret values are present and readable")
								}
							}
						}
					}
					cancel()
				}
			}
			if !ready {
				return errors.New("application is not ready")
			}
			return nil
		},
	}
}

func appDoctorCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "doctor",
		Summary: "Diagnose an application's runtime",
		Usage:   "baha app doctor [NAME]",
		Long:    "Checks desired state, local runtime files, Compose configuration, service state, authenticated protocol readiness and managed secret health. Without NAME it resolves the nearest repository baseharbor.yaml.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			resolved, err := resolveApplication(store, args, "doctor")
			if err != nil {
				return err
			}
			m := resolved.Manifest
			files, runtimeErr := application.ExistingRuntimeFiles(resolved.Store, m)
			var compose bhruntime.Compose
			var running []string
			var platformFiles bhruntime.Files
			var requiredStatuses []openbao.RequiredSecretStatus
			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			checks := []preflight.Check{
				{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
				{Name: "supported desired services", Run: func(context.Context) error { return application.CheckSupportedRuntimeServices(m) }},
				{Name: "manifest permissions", Run: func(context.Context) error {
					return checkManifestPermissions(resolved.ManifestPath, resolved.FromRepository)
				}},
				{Name: "runtime state", Run: func(context.Context) error { return runtimeErr }},
				{Name: "runtime permissions", Run: func(context.Context) error {
					if runtimeErr != nil {
						return runtimeErr
					}
					return application.CheckRuntimePermissions(files)
				}},
				{Name: "managed runtime definition", Run: func(context.Context) error {
					if runtimeErr != nil {
						return runtimeErr
					}
					return application.CheckManagedRuntimeDefinition(files, m)
				}},
				{Name: "container runtime + compose", Run: func(ctx context.Context) error {
					var err error
					compose, err = bhruntime.DetectCompose(ctx)
					return err
				}},
				{Name: "compose configuration", Run: func(ctx context.Context) error {
					if runtimeErr != nil {
						return runtimeErr
					}
					return compose.ConfigProject(ctx, application.RuntimeProjectName(m), files.Compose, files.Env)
				}},
				{Name: "running services", Run: func(ctx context.Context) error {
					if runtimeErr != nil {
						return runtimeErr
					}
					var err error
					running, err = compose.RunningServicesProject(ctx, application.RuntimeProjectName(m), files.Compose, files.Env)
					return err
				}},
			}
			if m.Services.Postgres {
				checks = append(checks,
					preflight.Check{Name: "postgres running", Run: func(context.Context) error {
						if !containsString(running, "postgres") {
							return errors.New("no postgres instance is running")
						}
						return nil
					}},
					preflight.Check{Name: "postgres readiness", Run: func(ctx context.Context) error {
						if !containsString(running, "postgres") {
							return errors.New("no postgres instance is running")
						}
						return application.VerifyPostgresRuntime(ctx, compose, m, files)
					}},
				)
			}
			if m.Services.Redis {
				checks = append(checks,
					preflight.Check{Name: "valkey running", Run: func(context.Context) error {
						if !containsString(running, "valkey") {
							return errors.New("no valkey instance is running")
						}
						return nil
					}},
					preflight.Check{Name: "valkey readiness", Run: func(ctx context.Context) error {
						if !containsString(running, "valkey") {
							return errors.New("no valkey instance is running")
						}
						return application.VerifyValkeyRuntime(ctx, compose, m, files)
					}},
				)
			}
			if m.Services.Secrets {
				checks = append(checks,
					preflight.Check{Name: "OpenBao control-plane runtime", Run: func(context.Context) error {
						var err error
						platformFiles, err = bhruntime.ExistingFiles("")
						return err
					}},
					preflight.Check{Name: "OpenBao application scope", Run: func(ctx context.Context) error {
						if runtimeErr != nil {
							return runtimeErr
						}
						if platformFiles.Compose == "" {
							return errors.New("BaseHarbor OpenBao runtime is not materialized")
						}
						identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
						return openbao.InspectApplicationScope(ctx, compose, platformFiles, identity, openbao.ApplicationCredentialsPath(files.Dir))
					}},
				)
				if len(application.RequiredSecretNames(m)) > 0 {
					checks = append(checks, preflight.Check{Name: "required application secrets", Run: func(ctx context.Context) error {
						if runtimeErr != nil {
							return runtimeErr
						}
						var err error
						requiredStatuses, err = inspectRequiredApplicationSecrets(ctx, compose, platformFiles, m, files)
						if err != nil {
							return err
						}
						return openbao.RequireApplicationSecrets(requiredStatuses)
					}})
				}
			}
			results, ok := preflight.Run(checkCtx, checks)
			preflight.Format(out, results)
			printRequiredSecretStatus(out, requiredStatuses)
			if !ok {
				return errors.New("application doctor found one or more failures")
			}
			fmt.Fprintln(out, "Application runtime is healthy.")
			return nil
		},
	}
}

func ownerOnly(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s is accessible by group or others (%o)", path, info.Mode().Perm())
	}
	return nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == wanted || strings.HasPrefix(value, wanted+"-") {
			return true
		}
	}
	return false
}
