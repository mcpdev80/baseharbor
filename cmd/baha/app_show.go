package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
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
	TelemetryState  string
	LastRecovery    *application.RecoveryMetadata `json:"last_recovery,omitempty"`
	LastBackup      *application.BackupMetadata
}

func appShowCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "show",
		Summary: "Show a coherent application overview",
		Usage:   "baha app show [NAME]",
		Long:    "Shows application identity, backend readiness, repository workload state, secret readiness and the last recorded successful backup without revealing secret values or credential-bearing URLs. It uses the same repository workload readiness model as app status and app doctor. Applications that have not been applied yet are shown as NOT READY instead of failing the inspection.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "app show")
			if err != nil {
				return err
			}
			args = filtered
			resolved, err := resolveApplication(ctx, store, args, "show")
			if err != nil {
				return err
			}
			overview, err := inspectApplicationOverview(ctx, resolved)
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, overview)
			}
			formatApplicationOverview(out, overview)
			return nil
		},
	}
}

func inspectApplicationOverview(ctx context.Context, resolved resolvedApplication) (applicationOverview, error) {
	if err := authorizeApplicationOperation(ctx, "app.show", resolved); err != nil {
		return applicationOverview{}, err
	}
	m := resolved.Manifest
	overview := applicationOverview{
		Name:           m.Name,
		Environment:    m.Environment,
		Ready:          true,
		SecretsState:   "not declared",
		BrokerState:    "not declared",
		TelemetryState: "not declared",
	}
	if resolved.FromRepository {
		overview.ManifestPath = resolved.ManifestPath
	}
	if err := application.CheckSupportedRuntimeServices(m); err != nil {
		return overview, err
	}
	lastBackup, backupErr := resolved.Store.LastBackup(m.Name)
	if backupErr == nil {
		if lastBackup.Environment == m.Environment {
			overview.LastBackup = &lastBackup
		}
	} else if !errors.Is(backupErr, application.ErrNoBackupMetadata) {
		return overview, backupErr
	}

	lastRecovery, recoveryErr := resolved.Store.LastRecovery(m.Name)
	if recoveryErr == nil && lastRecovery.Environment == m.Environment {
		overview.LastRecovery = &lastRecovery
	} else if recoveryErr != nil && !errors.Is(recoveryErr, application.ErrNoRecoveryMetadata) {
		return overview, recoveryErr
	}
	for _, name := range application.SQLInstanceNames(m) {
		overview.Postgres = append(overview.Postgres, overviewResource{Name: name, State: "not applied"})
	}
	for _, name := range application.ValkeyInstanceNames(m) {
		overview.Valkey = append(overview.Valkey, overviewResource{Name: name, State: "not applied"})
	}
	if application.HasOTLPTelemetry(m) {
		overview.TelemetryState = "not applied"
	}
	if m.Services.Secrets {
		overview.SecretsDeclared = true
		overview.SecretsRequired = len(application.RequiredSecretNames(m))
		overview.SecretsState = "not applied"
		overview.BrokerState = "not applied"
	}

	status, workload, err := collectResolvedApplicationStatus(ctx, resolved)
	if err != nil {
		return overview, err
	}
	projectApplicationOverviewStatus(&overview, status, workload)

	return overview, nil
}

// Project the canonical status observations; provider placement and literal
// service names in the Application project never decide overview readiness.
func projectApplicationOverviewStatus(overview *applicationOverview, status application.StatusResult, workload repositoryWorkloadStatus) {
	overview.Ready = status.Ready
	overview.Workload = workload
	state := func(name string) string {
		for _, check := range status.Checks {
			if check.Name == name {
				if check.OK {
					return "healthy"
				}
				return "not ready"
			}
		}
		if status.State == "not_applied" {
			return "not applied"
		}
		if status.State == "stopped" {
			return "not running"
		}
		return "unverified"
	}
	setOverviewResourceState(overview.Postgres, state("postgres"))
	setOverviewResourceState(overview.Valkey, state("valkey"))
	if overview.TelemetryState != "not declared" {
		overview.TelemetryState = state("telemetry/otlp")
	}
	if overview.SecretsDeclared {
		overview.SecretsState = state("secrets")
		overview.BrokerState = state("runtime-broker")
		for _, check := range status.Checks {
			if check.Name == "required-secrets" && !check.OK {
				overview.SecretsState = "not ready"
			}
			if strings.HasPrefix(check.Name, "required-secret/") {
				if check.OK {
					overview.SecretsReady++
				} else {
					overview.SecretsState = "not ready"
				}
			}
		}
	}
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

	fmt.Fprintln(out, "\nTelemetry")
	fmt.Fprintf(out, "  OTLP export           %s\n", overview.TelemetryState)

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
	if overview.LastBackup == nil {
		fmt.Fprintln(out, "  not recorded yet")
		return
	}
	fmt.Fprintf(out, "  created              %s\n", overview.LastBackup.CreatedAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(out, "  archive              %s\n", overview.LastBackup.ArchivePath)
	if len(overview.LastBackup.PostgresResources) > 0 {
		fmt.Fprintf(out, "  PostgreSQL           %s\n", strings.Join(overview.LastBackup.PostgresResources, ", "))
	} else {
		fmt.Fprintln(out, "  PostgreSQL           none")
	}
	if overview.LastBackup.IncludesSecrets {
		fmt.Fprintln(out, "  managed secrets      included")
	} else {
		fmt.Fprintln(out, "  managed secrets      not included")
	}
}
