package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

var appApplySecretInput io.Reader = os.Stdin
var appApplySecretReadHidden = readApplicationSecretFromTerminal
var appApplySecretIsTerminal = appInitReaderIsTerminal

func appApplyCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "apply",
		Summary: "Converge and verify an application's backend runtime",
		Usage:   "baha app apply [NAME]",
		Long:    "Runs plan, preflight, apply and verification. Without NAME it resolves the nearest baseharbor.yaml in the current repository, synchronizes a protected internal copy for runtime services, and treats the repository manifest as the source of truth. When an unambiguous application Compose workload exists, BaseHarbor generates a protected override, attaches it to the application backend network when managed backend services exist and injects container-routable native service URLs. Workload-only applications remain valid without inventing a managed database or cache. Declared secrets.required entries are readiness gates. Explicit secrets.required[].generate entries are created only when missing and are stored directly in OpenBao without printing their values. Managed-secret workloads start only after the per-application mTLS broker has proven app-scoped OpenBao readiness.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return executeApplicationApplyLifecycle(ctx, store, args, out, errOut)
		},
	}
}

func executeApplicationApplyLifecycle(ctx context.Context, store application.Store, args []string, out, errOut io.Writer) error {
	execution, err := newApplicationApplyExecution(ctx, store, args, out, errOut)
	if err != nil {
		return err
	}
	if err := execution.runPreflight(ctx); err != nil {
		return err
	}
	if err := execution.prepareManagedRuntime(ctx); err != nil {
		return err
	}
	if err := execution.verifyManagedRuntime(ctx); err != nil {
		return err
	}
	if err := execution.convergeApplicationRuntime(ctx); err != nil {
		return err
	}
	return execution.recordVerifiedDeployment(ctx)
}

type applicationSecretSetter interface {
	Set(context.Context, string, string, []byte) error
}

func resolveMissingRequiredSecretsInteractive(
	ctx context.Context,
	service applicationSecretSetter,
	compose bhruntime.Compose,
	platformFiles bhruntime.Files,
	m application.Manifest,
	files application.RuntimeFiles,
	out io.Writer,
) error {
	statuses, err := inspectRequiredApplicationSecrets(ctx, compose, platformFiles, m, files)
	if err != nil {
		return fmt.Errorf("inspect required application secrets: %w", err)
	}
	var missing []openbao.RequiredSecretStatus
	for _, status := range statuses {
		if status.Generated || (status.Present && status.Usable) {
			continue
		}
		missing = append(missing, status)
	}
	if len(missing) == 0 {
		return nil
	}

	return promptAndStoreMissingRequiredSecrets(ctx, service, m.Name, missing, out)
}

func promptAndStoreMissingRequiredSecrets(
	ctx context.Context,
	service applicationSecretSetter,
	applicationName string,
	missing []openbao.RequiredSecretStatus,
	out io.Writer,
) error {
	if len(missing) == 0 {
		return nil
	}
	if noInput(ctx) || !appApplySecretIsTerminal(appApplySecretInput) {
		return &machine.Error{
			Code:        machine.ErrorRequiredSecretMissing,
			CauseCode:   "required_secret_missing",
			Message:     "Required application secret " + missing[0].Name + " is missing.",
			Resource:    missing[0].Name,
			Remediation: "requires developer input",
			Next:        "Run 'baha app secret set " + missing[0].Name + "' interactively or use --stdin for automation.",
		}
	}

	fmt.Fprintln(out, "\nMissing required application secrets")
	for _, status := range missing {
		fmt.Fprintf(out, "  %s\n", status.Name)
	}
	reader := bufio.NewReader(appApplySecretInput)
	confirmed, err := promptYesNo(reader, out, "Configure now?", true)
	if err != nil {
		return err
	}
	if !confirmed {
		return &machine.Error{
			Code:        machine.ErrorRequiredSecretMissing,
			CauseCode:   "required_secret_missing",
			Message:     "Required application secret " + missing[0].Name + " remains unresolved.",
			Resource:    missing[0].Name,
			Remediation: "requires developer input",
			Next:        "Run 'baha app secret set " + missing[0].Name + "' and retry.",
		}
	}

	for _, status := range missing {
		value, err := appApplySecretReadHidden(appApplySecretInput, out, status.Name)
		if err != nil {
			return err
		}
		if err := service.Set(ctx, applicationName, status.Name, value); err != nil {
			zeroBytes(value)
			return fmt.Errorf("store application secret %s: %w", status.Name, err)
		}
		zeroBytes(value)
		fmt.Fprintf(out, "[OK] secret %s stored securely\n", status.Name)
	}
	return nil
}

func startManagedRuntime(ctx context.Context, out io.Writer, compose bhruntime.Compose, m application.Manifest, files application.RuntimeFiles) error {
	if !application.HasManagedRuntimeServices(m) {
		return nil
	}
	const maxAttempts = 3
	project := files.Project

	providerOverride, providerLogging, err := logsprovider.ExistingProviderSourceOverride(files)
	if err != nil {
		return err
	}
	composeFiles := []string{files.Compose}
	if providerLogging {
		composeFiles = append(composeFiles, providerOverride)
	}
	environment, err := application.RuntimeEnvironment(files)
	if err != nil {
		return err
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := compose.UpProjectFilesSelectedProgress(ctx, project, files.Dir, environment, nil, func(detail string) {
			cli.ReportActivityDetail(out, detail)
		}, composeFiles...)
		if err == nil {
			return nil
		}
		if !bhruntime.IsPortBindingConflict(err) || attempt == maxAttempts {
			return err
		}

		if downErr := compose.DownProjectFilesEnv(ctx, project, files.Dir, environment, composeFiles...); downErr != nil {
			return errors.Join(err, fmt.Errorf("clean up partially started runtime before host-port retry: %w", downErr))
		}
		if reallocErr := application.ReallocateRuntimePorts(m, files); reallocErr != nil {
			return errors.Join(err, fmt.Errorf("reallocate application host ports: %w", reallocErr))
		}
		environment, err = application.RuntimeEnvironment(files)
		if err != nil {
			return errors.Join(err, fmt.Errorf("reload application runtime environment after host-port reallocation: %w", err))
		}
		if configErr := compose.ConfigProjectFilesEnv(ctx, project, files.Dir, environment, composeFiles...); configErr != nil {
			return errors.Join(err, fmt.Errorf("validate runtime after host-port reallocation: %w", configErr))
		}
		fmt.Fprintf(out, "[RETRY] host-port conflict detected; reassigned loopback ports (attempt %d/%d)\n", attempt+1, maxAttempts)
	}
	return errors.New("application runtime start exhausted host-port retries")
}

func verifyDesiredRuntimeServices(ctx context.Context, compose bhruntime.Compose, m application.Manifest, files application.RuntimeFiles) error {
	if m.Services.SQL {
		if err := application.VerifyPostgresRuntime(ctx, compose, m, files); err != nil {
			return err
		}
	}
	if m.Services.Cache {
		if err := application.VerifyValkeyRuntime(ctx, compose, m, files); err != nil {
			return err
		}
	}
	return nil
}

func renderRuntimeReady(term *cli.Terminal, m application.Manifest) {
	term.Section("Services")
	if m.Services.SQL {
		term.Result("READY", "PostgreSQL", "authenticated SELECT 1")
	}
	if m.Services.Cache {
		term.Result("READY", "Valkey", "authenticated PING")
	}
	if m.Services.Secrets {
		term.Result("VERIFIED", "secrets", "isolated OpenBao application scope")
		term.Result("READY", "runtime-broker", "mTLS identity verified")
		if len(m.Secrets.Required) > 0 {
			term.Result("VERIFIED", "required-secrets", fmt.Sprintf("%d declared secret(s) present", len(m.Secrets.Required)))
		}
	}
}
