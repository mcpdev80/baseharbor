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
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
)

const (
	applicationStatusTimeout         = 120 * time.Second
	applicationOpenBaoStatusTimeout  = 30 * time.Second
	applicationRequiredSecretTimeout = 40 * time.Second
	applicationBrokerStatusTimeout   = 10 * time.Second
	applicationPostgresStatusTimeout = 15 * time.Second
	applicationValkeyStatusTimeout   = 10 * time.Second
	applicationLogsStatusTimeout     = 10 * time.Second
)

func collectApplicationStatus(ctx context.Context, store application.Store, args []string) (application.StatusResult, error) {
	statusCtx, cancelStatus := context.WithTimeout(ctx, applicationStatusTimeout)
	defer cancelStatus()

	collection, done, err := newApplicationStatusCollection(statusCtx, store, args)
	if err != nil || done {
		return collection.result, err
	}
	if collection.componentsStopped(statusCtx) {
		collection.result.State = "stopped"
		collection.result.Ready = false
		return collection.result, nil
	}
	collection.collectManagedServiceChecks(statusCtx)
	collection.collectWorkloadChecks()
	collection.collectLogsCheck(statusCtx)
	collection.collectExposureCheck(statusCtx)
	return collection.result, nil
}

func renderApplicationStatus(ctx context.Context, out, errOut io.Writer, result application.StatusResult) {
	renderApplicationStatusWithExtra(ctx, out, errOut, result, nil)
}

func renderApplicationStatusWithExtra(ctx context.Context, out, errOut io.Writer, result application.StatusResult, extra func(*cli.Terminal)) {
	term := cli.NewTerminal(ctx, out, errOut)
	term.Header(result.Application, result.Environment)
	term.Section("Application")
	if result.State == "not_applied" {
		term.Result("NOT APPLIED", "application", "no BaseHarbor-managed runtime state exists")
		fmt.Fprintln(out, "\nNOT APPLIED")
		fmt.Fprintln(out, "\nNext:")
		fmt.Fprintln(out, "  baha up")
		fmt.Fprintln(out, "  baha app apply")
		return
	}
	if result.State == "stopped" {
		term.Result("STOPPED", "application", "persistent application state preserved")
		fmt.Fprintln(out, "\nSTOPPED")
		return
	}

	term.Result(map[bool]string{true: "READY", false: "DEGRADED"}[result.Ready], "application", map[bool]string{true: "all requested components verified", false: "one or more components need attention"}[result.Ready])

	sections := map[string][]application.StatusCheck{}
	order := []string{"Services", "Workload", "Observability", "Exposure", "Other"}
	for _, check := range result.Checks {
		section := "Other"
		switch {
		case strings.HasPrefix(check.Name, "postgres"), strings.HasPrefix(check.Name, "valkey"), strings.HasPrefix(check.Name, "secrets"), strings.HasPrefix(check.Name, "runtime-broker"), strings.HasPrefix(check.Name, "object-storage"), strings.HasPrefix(check.Name, "required-secret"):
			section = "Services"
		case strings.HasPrefix(check.Name, "workload"):
			section = "Workload"
		case strings.HasPrefix(check.Name, "logs"), strings.HasPrefix(check.Name, "telemetry"), strings.HasPrefix(check.Name, "traces"), strings.HasPrefix(check.Name, "metrics"):
			section = "Observability"
		case strings.Contains(check.Name, "exposure"):
			section = "Exposure"
		}
		sections[section] = append(sections[section], check)
	}
	for _, section := range order {
		checks := sections[section]
		if len(checks) == 0 {
			continue
		}
		term.Section(section)
		for _, check := range checks {
			state := "READY"
			if section == "Observability" {
				state = "VERIFIED"
			}
			if !check.OK {
				state = "FAILED"
			}
			term.Result(state, check.Name, statusHumanDetail(term, check))
		}
	}
	if extra != nil {
		extra(term)
	}

	if result.Ready {
		fmt.Fprintln(out, "\nREADY")
	} else {
		fmt.Fprintln(out, "\nDEGRADED")
		fmt.Fprintln(out, "\nNext:")
		fmt.Fprintln(out, "  baha doctor")
		fmt.Fprintln(out, "  baha status --verbose")
	}
}

func statusHumanDetail(term *cli.Terminal, check application.StatusCheck) string {
	return statusHumanDetailValue(check, term.Verbose())
}

func statusHumanDetailValue(check application.StatusCheck, verbose bool) string {
	if verbose || check.OK {
		if !verbose {
			switch check.Name {
			case "postgres":
				return "authenticated and ready"
			case "valkey":
				return "authenticated and ready"
			case "secrets":
				return "OpenBao application scope ready"
			case "runtime-broker":
				return "mTLS readiness verified"
			case "managed-exposure":
				return "configured endpoint(s) ready"
			}
		}
		return check.Detail
	}

	switch {
	case check.Name == "secrets":
		return "OpenBao application scope unavailable"
	case check.Name == "runtime-broker":
		return "runtime broker is not ready"
	case check.Name == "workload" && strings.Contains(strings.ToLower(check.Detail), "openbao"):
		return "required secrets unavailable because OpenBao is not running"
	case strings.HasPrefix(check.Name, "workload/"):
		return "workload service is not ready"
	case check.Name == "logs":
		return "log ingestion is not ready"
	case check.Name == "telemetry/otlp":
		return "telemetry endpoint is not ready"
	case check.Name == "object-storage":
		return "object storage is not ready"
	default:
		return check.Detail
	}
}

func appStatusCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "status",
		Summary: "Show application runtime and readiness status",
		Usage:   "baha app status [NAME] [-o json|--output json]",
		Long:    "Reports materialized backend state, repository workload state and protocol-level readiness. Human and structured output are rendered from the same secret-safe readiness result.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "app status")
			if err != nil {
				return err
			}
			result, err := collectApplicationStatus(ctx, store, filtered)
			if err != nil {
				return err
			}
			if format == outputJSON {
				if err := writeJSON(out, result); err != nil {
					return err
				}
			} else {
				renderApplicationStatus(ctx, out, errOut, result)
			}
			if result.State == "stopped" || result.State == "not_applied" {
				return nil
			}
			if !result.Ready {
				return cli.Presented(errors.New("application is not ready"))
			}
			return nil
		},
	}
}

func appDoctorCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "doctor",
		Summary: "Diagnose an application's runtime",
		Usage:   "baha app doctor [NAME] [-o json|--output json]",
		Long:    "Checks desired state, local runtime files, runtime configuration, backend service state, repository workload state, authenticated protocol readiness, managed OpenBao secret scope health, per-application mTLS broker readiness and required-secret presence/usability. Without NAME it resolves the nearest repository baseharbor.yaml.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "app doctor")
			if err != nil {
				return err
			}
			result, err := collectApplicationDoctor(ctx, store, filtered)
			if err != nil {
				return err
			}
			if format == outputJSON {
				if err := writeJSON(out, result); err != nil {
					return err
				}
			} else {
				renderCollectedApplicationDoctor(ctx, out, errOut, result)
			}
			if result.State == "not_applied" {
				return nil
			}
			if !result.Healthy {
				return cli.Presented(errors.New("application doctor found one or more failures"))
			}
			return nil
		},
	}
}

func renderCollectedApplicationDoctor(ctx context.Context, out, errOut io.Writer, result applicationDoctorResult) {
	if result.State == "not_applied" {
		term := cli.NewTerminal(ctx, out, errOut)
		term.Header(result.Application, result.Environment)
		term.Section("Application")
		term.Result("NOT APPLIED", "application", "no BaseHarbor-managed runtime state exists")
		fmt.Fprintln(out, "\nNext:")
		fmt.Fprintln(out, "  baha up")
		fmt.Fprintln(out, "  baha app apply")
		fmt.Fprintln(out, "\nNOT APPLIED")
		return
	}
	renderApplicationDoctor(
		ctx,
		out,
		errOut,
		result.manifest,
		result.Checks,
		result.workloadStatus,
		result.requiredSecretStatuses,
		result.workloadSecurity,
		result.Healthy,
		result.ServiceTLS,
		result.serviceTLSErr,
		result.tlsStatus,
		result.tlsErr,
	)
}

func renderApplicationDoctor(
	ctx context.Context,
	out io.Writer,
	errOut io.Writer,
	m application.Manifest,
	results []preflight.Result,
	workload repositoryWorkloadStatus,
	requiredSecrets []openbao.RequiredSecretStatus,
	workloadSecurity application.WorkloadSecurityReport,
	healthy bool,
	serviceTLS []application.BackendTLSLifecycleObservation,
	serviceTLSErr error,
	tlsStatus *applicationTLSStatus,
	tlsErr error,
) {
	term := cli.NewTerminal(ctx, out, errOut)
	term.Header(m.Name, m.Environment)

	sections := map[string][]preflight.Result{}
	order := []string{"Core", "Services", "Workload", "Observability", "Exposure"}
	for _, result := range results {
		section := applicationDoctorSection(result.Name)
		sections[section] = append(sections[section], result)
	}
	for _, section := range order {
		items := sections[section]
		if len(items) == 0 {
			continue
		}
		term.Section(section)
		for _, result := range items {
			state := "OK"
			if !result.OK {
				state = "FAILED"
			}
			detail := ""
			if !result.OK || term.Verbose() {
				detail = doctorHumanDetail(term, result)
			}
			term.Result(state, result.Name, detail)
		}
	}

	if workload.Found && len(workload.Services) > 0 {
		term.Section("Workload services")
		for _, service := range workload.Services {
			state := "READY"
			if !service.Ready {
				state = "FAILED"
			}
			term.Result(state, service.Service, formatWorkloadServiceStatus(service))
		}
	}

	if len(requiredSecrets) > 0 {
		term.Section("Required secrets")
		for _, status := range requiredSecrets {
			state := "READY"
			detail := "present and usable"
			if !status.Present || !status.Usable {
				state = "MISSING"
				detail = "missing or unusable"
			} else if status.Generated {
				detail = "present and usable · managed generation enabled"
			}
			term.Result(state, status.Name, detail)
		}
	}

	if term.Verbose() {
		printWorkloadSecurityFindings(out, workloadSecurity)
	}
	if serviceTLSErr != nil {
		term.Section("Service TLS")
		term.Result("FAILED", "lifecycle", serviceTLSErr.Error())
	} else {
		renderServiceTLSLifecycle(term, serviceTLS)
	}
	if tlsStatus != nil || tlsErr != nil {
		term.Section("TLS")
		if tlsErr != nil {
			term.Result("FAILED", "certificate", conciseTLSStatusError(tlsErr))
		} else {
			renderApplicationTLSStatus(term, *tlsStatus)
		}
	}

	if healthy && serviceTLSErr == nil && tlsErr == nil {
		fmt.Fprintln(out, "\nREADY")
		return
	}
	fmt.Fprintln(out, "\nNext:")
	fmt.Fprintln(out, "  baha status --verbose")
	fmt.Fprintln(out, "  baha doctor --verbose")
	fmt.Fprintln(out, "  baha app doctor --fix")
	fmt.Fprintln(out, "\nDEGRADED · one or more checks require attention")
}

func doctorHumanDetail(term *cli.Terminal, result preflight.Result) string {
	return doctorHumanDetailValue(result, term.Verbose())
}

func doctorHumanDetailValue(result preflight.Result, verbose bool) string {
	if verbose || result.OK {
		return result.Detail
	}
	lowerName := strings.ToLower(result.Name)
	lowerDetail := strings.ToLower(result.Detail)
	switch {
	case strings.Contains(lowerName, "managed runtime definition"):
		return "runtime definition differs from BaseHarbor-managed state"
	case strings.Contains(lowerName, "openbao application scope"):
		return "OpenBao application scope unavailable"
	case strings.Contains(lowerName, "application runtime broker"):
		return "runtime broker is not ready"
	case strings.Contains(lowerName, "required application secrets"):
		return "required secrets could not be verified because OpenBao is unavailable"
	case strings.Contains(lowerName, "workload security") && strings.Contains(lowerDetail, "required variable"):
		if strings.Contains(lowerDetail, "secret_key") {
			return "workload requires a BaseHarbor/OpenBao managed secret before security preflight can complete"
		}
		return "workload requires a missing environment value before security preflight can complete"
	case strings.Contains(lowerName, "repository workload") && strings.Contains(lowerDetail, "openbao"):
		return "workload cannot resolve required secrets because OpenBao is unavailable"
	default:
		return result.Detail
	}
}

func applicationDoctorSection(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "postgres"),
		strings.Contains(lower, "valkey"),
		strings.Contains(lower, "openbao"),
		strings.Contains(lower, "secret"),
		strings.Contains(lower, "broker"),
		strings.Contains(lower, "object-storage"),
		strings.Contains(lower, "running services"):
		return "Services"
	case strings.Contains(lower, "workload"):
		return "Workload"
	case strings.Contains(lower, "log"),
		strings.Contains(lower, "telemetry"),
		strings.Contains(lower, "otlp"),
		strings.Contains(lower, "metric"),
		strings.Contains(lower, "trace"):
		return "Observability"
	case strings.Contains(lower, "exposure"),
		strings.Contains(lower, "http"),
		strings.Contains(lower, "tls"):
		return "Exposure"
	default:
		return "Core"
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

func applicationComponentsStopped(managedRunning, workloadRunning []string, workloadFound, brokerRunning, exposureRunning bool) bool {
	if len(managedRunning) != 0 || brokerRunning || exposureRunning {
		return false
	}
	if workloadFound && len(workloadRunning) != 0 {
		return false
	}
	return true
}
