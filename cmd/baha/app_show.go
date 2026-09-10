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
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type overviewResource struct {
	Name  string
	State string
}

type applicationOverview struct {
	Name            string
	Environment     string
	ManifestPath    string
	Ready           bool
	Postgres        []overviewResource
	Valkey          []overviewResource
	Workload        repositoryWorkloadStatus
	SecretsDeclared bool
	SecretsRequired int
	SecretsReady    int
	SecretsState    string
	BrokerState     string
}

func appShowCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "show",
		Summary: "Show a coherent application overview",
		Usage:   "baha app show [NAME]",
		Long:    "Shows application identity, backend readiness, repository workload state and secret readiness without revealing secret values or credential-bearing URLs. It uses the same repository workload readiness model as app status and app doctor. Applications that have not been applied yet are shown as NOT READY instead of failing the inspection.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			resolved, err := resolveApplication(store, args, "show")
			if err != nil {
				return err
			}
			overview, err := inspectApplicationOverview(ctx, resolved)
			if err != nil {
				return err
			}
			formatApplicationOverview(out, overview)
			return nil
		},
	}
}

func inspectApplicationOverview(ctx context.Context, resolved resolvedApplication) (applicationOverview, error) {
	m := resolved.Manifest
	overview := applicationOverview{
		Name:         m.Name,
		Environment:  m.Environment,
		Ready:        true,
		SecretsState: "not declared",
		BrokerState:  "not declared",
	}
	if resolved.FromRepository {
		overview.ManifestPath = resolved.ManifestPath
	}
	if err := application.CheckSupportedRuntimeServices(m); err != nil {
		return overview, err
	}

	for _, name := range application.PostgresInstanceNames(m) {
		overview.Postgres = append(overview.Postgres, overviewResource{Name: name, State: "not applied"})
	}
	for _, name := range application.RedisInstanceNames(m) {
		overview.Valkey = append(overview.Valkey, overviewResource{Name: name, State: "not applied"})
	}
	if m.Services.Secrets {
		overview.SecretsDeclared = true
		overview.SecretsRequired = len(application.RequiredSecretNames(m))
		overview.SecretsState = "not applied"
		overview.BrokerState = "not applied"
	}

	files, err := application.ExistingRuntimeFiles(resolved.Store, m)
	if errors.Is(err, application.ErrRuntimeNotApplied) {
		overview.Ready = false
		return overview, nil
	}
	if err != nil {
		return overview, err
	}
	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		return overview, err
	}
	running, err := compose.RunningServicesProject(ctx, application.RuntimeProjectName(m), files.Compose, files.Env)
	if err != nil {
		return overview, err
	}

	if m.Services.Postgres {
		state := "not running"
		if containsString(running, "postgres") {
			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			verifyErr := application.VerifyPostgresRuntime(checkCtx, compose, m, files)
			cancel()
			if verifyErr == nil {
				state = "healthy"
			} else {
				state = "not ready"
			}
		}
		setOverviewResourceState(overview.Postgres, state)
		if state != "healthy" {
			overview.Ready = false
		}
	}

	if m.Services.Redis {
		state := "not running"
		if containsString(running, "valkey") {
			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			verifyErr := application.VerifyValkeyRuntime(checkCtx, compose, m, files)
			cancel()
			if verifyErr == nil {
				state = "healthy"
			} else {
				state = "not ready"
			}
		}
		setOverviewResourceState(overview.Valkey, state)
		if state != "healthy" {
			overview.Ready = false
		}
	}

	if m.Services.Secrets {
		overview.SecretsState = "not ready"
		overview.BrokerState = "not ready"
		platformFiles, platformErr := bhruntime.ExistingFiles("")
		if platformErr == nil {
			checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
			if openbao.InspectApplicationScope(checkCtx, compose, platformFiles, identity, openbao.ApplicationCredentialsPath(files.Dir)) == nil {
				overview.SecretsState = "healthy"
				statuses, statusErr := inspectRequiredApplicationSecrets(checkCtx, compose, platformFiles, m, files)
				if statusErr == nil {
					for _, status := range statuses {
						if status.Present && status.Usable {
							overview.SecretsReady++
						}
					}
					if openbao.RequireApplicationSecrets(statuses) != nil {
						overview.SecretsState = "not ready"
					}
				}
			}
			cancel()
		}
		brokerCtx, brokerCancel := context.WithTimeout(ctx, 10*time.Second)
		if verifyRuntimeBrokerRunning(brokerCtx, compose, m, files) == nil {
			overview.BrokerState = "healthy"
		}
		brokerCancel()
		if overview.SecretsState != "healthy" || overview.BrokerState != "healthy" {
			overview.Ready = false
		}
	}

	workloadStatus, workloadErr := inspectRepositoryWorkloadStatus(ctx, compose, resolved, files)
	overview.Workload = workloadStatus
	if workloadErr != nil || (workloadStatus.Found && !workloadStatus.Ready()) {
		overview.Ready = false
	}

	return overview, nil
}

func setOverviewResourceState(resources []overviewResource, state string) {
	for i := range resources {
		resources[i].State = state
	}
}

func formatApplicationOverview(out io.Writer, overview applicationOverview) {
	status := "NOT READY"
	if overview.Ready {
		status = "READY"
	}
	fmt.Fprintf(out, "Application: %s\n", overview.Name)
	fmt.Fprintf(out, "Environment: %s\n", overview.Environment)
	fmt.Fprintf(out, "Status: %s\n", status)
	if overview.ManifestPath != "" {
		fmt.Fprintf(out, "Manifest: %s\n", overview.ManifestPath)
	}

	fmt.Fprintln(out, "\nBackends")
	if len(overview.Postgres) == 0 && len(overview.Valkey) == 0 {
		fmt.Fprintln(out, "  none declared")
	}
	for _, resource := range overview.Postgres {
		fmt.Fprintf(out, "  PostgreSQL %-16s %s\n", resource.Name, resource.State)
	}
	for _, resource := range overview.Valkey {
		fmt.Fprintf(out, "  Valkey     %-16s %s\n", resource.Name, resource.State)
	}

	fmt.Fprintln(out, "\nWorkload")
	if !overview.Workload.Found {
		fmt.Fprintln(out, "  none detected")
	} else {
		for _, service := range overview.Workload.Services {
			state := formatWorkloadServiceStatus(service)
			fmt.Fprintf(out, "  %-20s %s\n", service.Service, state)
		}
	}

	fmt.Fprintln(out, "\nSecrets")
	if !overview.SecretsDeclared {
		fmt.Fprintln(out, "  none declared")
	} else {
		fmt.Fprintf(out, "  required             %d\n", overview.SecretsRequired)
		fmt.Fprintf(out, "  ready                %d\n", overview.SecretsReady)
		fmt.Fprintf(out, "  scope                %s\n", overview.SecretsState)
		fmt.Fprintf(out, "  runtime broker       %s\n", overview.BrokerState)
	}

	fmt.Fprintln(out, "\nLast backup")
	fmt.Fprintln(out, "  not recorded yet")
}
