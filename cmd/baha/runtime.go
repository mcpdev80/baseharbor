package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/config"
	"github.com/mcpdev80/baseharbor/internal/connectivityrelay"
	"github.com/mcpdev80/baseharbor/internal/health"
	"github.com/mcpdev80/baseharbor/internal/hosttrust"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimeexecutor"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
	tracesprovider "github.com/mcpdev80/baseharbor/internal/traces"
)

var runtimeInput io.Reader = os.Stdin

type runtimeUpOptions struct {
	Yes              bool
	ControlPlaneOnly bool
	PostgresPort     int
	OpenBaoPort      int
	RecoveryFile     string
	Environment      string
	TrustHostCA      bool
}

func runtimeUpCommand(ctx context.Context, args []string, out, errOut io.Writer) error {
	opts, err := parseRuntimeUpOptions(args)
	if err != nil {
		return err
	}
	restoreEnvironment := pushApplicationEnvironmentOverride(opts.Environment)
	defer restoreEnvironment()
	if err := runtimeUpGuided(ctx, runtimeInput, out, opts); err != nil {
		return err
	}
	if opts.ControlPlaneOnly {
		return maybeOfferManagedHostTrustWhenReady(ctx, runtimeInput, out, opts)
	}
	return repositoryApplicationUp(ctx, runtimeInput, out, errOut, opts)
}

func parseRuntimeUpOptions(args []string) (runtimeUpOptions, error) {
	opts := runtimeUpOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--yes", "-y":
			opts.Yes = true
		case "--control-plane-only":
			opts.ControlPlaneOnly = true
		case "--trust-host-ca":
			opts.TrustHostCA = true
		case "--environment", "-e":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return opts, usageError("--environment requires ENV", "Example: baha up -e dev")
			}
			i++
			opts.Environment = strings.TrimSpace(args[i])
		case "--postgres-port":
			if i+1 >= len(args) {
				return opts, usageError("--postgres-port requires PORT", "Example: baha up --postgres-port 15432")
			}
			i++
			port, err := parsePort(args[i])
			if err != nil {
				return opts, err
			}
			opts.PostgresPort = port
		case "--openbao-port":
			if i+1 >= len(args) {
				return opts, usageError("--openbao-port requires PORT", "Example: baha up --openbao-port 18200")
			}
			i++
			port, err := parsePort(args[i])
			if err != nil {
				return opts, err
			}
			opts.OpenBaoPort = port
		case "--recovery-file":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return opts, usageError("--recovery-file requires PATH", "Example: baha up --recovery-file /secure/openbao-recovery.json")
			}
			i++
			opts.RecoveryFile = args[i]
		default:
			if strings.HasPrefix(args[i], "--environment=") {
				opts.Environment = strings.TrimSpace(strings.TrimPrefix(args[i], "--environment="))
				if opts.Environment == "" {
					return opts, usageError("--environment requires ENV", "Example: baha up --environment dev")
				}
				continue
			}
			if strings.HasPrefix(args[i], "--recovery-file=") {
				opts.RecoveryFile = strings.TrimPrefix(args[i], "--recovery-file=")
				if strings.TrimSpace(opts.RecoveryFile) == "" {
					return opts, usageError("--recovery-file requires PATH", "Example: baha up --recovery-file /secure/openbao-recovery.json")
				}
				continue
			}
			return opts, unknownOptionUsage("baha up", args[i], "--yes", "-y", "--control-plane-only", "--trust-host-ca", "--environment", "-e", "--postgres-port", "--openbao-port", "--recovery-file")
		}
	}
	return opts, nil
}

func parsePort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, usageError("invalid port "+value, "Choose a TCP port between 1 and 65535.")
	}
	return port, nil
}

func runtimeUpGuided(parent context.Context, in io.Reader, out io.Writer, opts runtimeUpOptions) error {
	if opts.PostgresPort != 0 && opts.OpenBaoPort != 0 && opts.PostgresPort == opts.OpenBaoPort {
		return errors.New("PostgreSQL and OpenBao cannot use the same host port")
	}
	if _, err := bhruntime.ExistingFiles(""); err == nil {
		if opts.PostgresPort != 0 || opts.OpenBaoPort != 0 {
			return usageError("control-plane ports cannot be changed through 'baha up' after initialization", "Edit the existing runtime deliberately or recreate the control plane instead.")
		}
		return runtimeUpExisting(parent, out, opts.RecoveryFile)
	}

	postgresPort, err := selectControlPlanePort(out, "PostgreSQL", "--postgres-port", opts.PostgresPort, bhruntime.DefaultPostgresPort, 15432)
	if err != nil {
		return err
	}
	openBaoPort, err := selectControlPlanePort(out, "OpenBao", "--openbao-port", opts.OpenBaoPort, bhruntime.DefaultOpenBaoPort, 18200)
	if err != nil {
		return err
	}
	if postgresPort == openBaoPort {
		return errors.New("PostgreSQL and OpenBao cannot use the same host port")
	}

	interactive := !opts.Yes && !noInput(parent) && readerIsTerminal(in)
	if interactive {
		reader := bufio.NewReader(in)
		fmt.Fprintln(out, "BaseHarbor control-plane setup")
		fmt.Fprintln(out)
		fmt.Fprintf(out, "PostgreSQL host port [%d]: ", postgresPort)
		selected, err := readPortChoice(reader, postgresPort)
		if err != nil {
			return err
		}
		postgresPort = selected
		fmt.Fprintf(out, "OpenBao host port [%d]: ", openBaoPort)
		selected, err = readPortChoice(reader, openBaoPort)
		if err != nil {
			return err
		}
		openBaoPort = selected
		if postgresPort == openBaoPort {
			return errors.New("PostgreSQL and OpenBao cannot use the same host port")
		}
		if !portAvailable(postgresPort) {
			return occupiedSelectedPortError("PostgreSQL", "--postgres-port", postgresPort, 15432)
		}
		if !portAvailable(openBaoPort) {
			return occupiedSelectedPortError("OpenBao", "--openbao-port", openBaoPort, 18200)
		}
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Configuration:")
		fmt.Fprintf(out, "  PostgreSQL  127.0.0.1:%d\n", postgresPort)
		fmt.Fprintf(out, "  OpenBao     127.0.0.1:%d\n", openBaoPort)
		fmt.Fprint(out, "Accept? [Y/n]: ")
		answer, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "" && answer != "y" && answer != "yes" && answer != "j" && answer != "ja" {
			return errors.New("control-plane setup cancelled")
		}
	} else {
		fmt.Fprintln(out, "BaseHarbor control-plane ports:")
		fmt.Fprintf(out, "  PostgreSQL  127.0.0.1:%d\n", postgresPort)
		fmt.Fprintf(out, "  OpenBao     127.0.0.1:%d\n", openBaoPort)
	}

	return runtimeUpWithPorts(parent, out, bhruntime.Ports{Postgres: postgresPort, OpenBao: openBaoPort})
}

func selectControlPlanePort(out io.Writer, service, flag string, requested, defaultPort, fallbackStart int) (int, error) {
	if requested != 0 {
		if portAvailable(requested) {
			return requested, nil
		}
		return 0, occupiedSelectedPortError(service, flag, requested, fallbackStart)
	}
	if portAvailable(defaultPort) {
		return defaultPort, nil
	}
	fallback := firstAvailablePort(fallbackStart)
	if fallback == 0 {
		return 0, fmt.Errorf("%s default host port %d is already in use and no free fallback port was found", service, defaultPort)
	}
	fmt.Fprintf(out, "%s host port %d is already in use.\n", service, defaultPort)
	fmt.Fprintf(out, "Found free loopback port %d; using it automatically.\n", fallback)
	return fallback, nil
}

func occupiedSelectedPortError(service, flag string, port, fallbackStart int) error {
	fallback := firstAvailablePort(fallbackStart)
	if fallback == 0 {
		return fmt.Errorf("%s host port %d is already in use; choose another free TCP port with 'baha up %s PORT'", service, port, flag)
	}
	return fmt.Errorf("%s host port %d is already in use; free port %d is available. Retry with 'baha up %s %d'", service, port, fallback, flag, fallback)
}

func readPortChoice(reader *bufio.Reader, fallback int) (int, error) {
	value, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	return parsePort(value)
}

func readerIsTerminal(in io.Reader) bool {
	file, ok := in.(*os.File)
	if !ok {
		return true
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func portAvailable(port int) bool {
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

func firstAvailablePort(start int) int {
	for port := start; port <= 65535; port++ {
		if portAvailable(port) {
			return port
		}
	}
	return 0
}

func runtimeUp(parent context.Context, out io.Writer) error {
	return runtimeUpExisting(parent, out, "")
}

func runtimeUpWithPorts(parent context.Context, out io.Writer, ports bhruntime.Ports) error {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()

	compose, files, err := startControlPlaneRuntime(ctx, out, ports)
	if err != nil {
		return err
	}
	if err := waitForOpenBaoExecReady(ctx, compose, files); err != nil {
		return fmt.Errorf("wait for OpenBao control-plane readiness: %w", err)
	}
	if state, inspectErr := platformopenbao.Inspect(ctx, compose, files); inspectErr == nil && state.Initialized && !state.Sealed {
		if managerErr := platformopenbao.CheckManager(ctx, compose, files); managerErr == nil {
			if err := reconcileControlPlaneServiceAccess(ctx, compose, files); err != nil {
				return fmt.Errorf("reconcile control-plane service access: %w", err)
			}
		}
	}
	if err := resumeSharedPlatformRuntime(ctx, compose, out); err != nil {
		return fmt.Errorf("resume shared platform runtime: %w", err)
	}
	fmt.Fprintln(out, "BaseHarbor control-plane runtime started")
	fmt.Fprintln(out, "next: run 'baha status' and 'baha doctor'")
	return nil
}

func waitForOpenBaoExecReady(ctx context.Context, compose bhruntime.Compose, files bhruntime.Files) error {
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for {
		if _, err := platformopenbao.Inspect(ctx, compose, files); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return lastErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func runtimeUpExisting(parent context.Context, out io.Writer, recoveryFile string) error {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()

	if strings.TrimSpace(recoveryFile) == "" {
		checks := health.RuntimeChecks()
		if len(checks) > 0 {
			_, ready := health.Format(checks)
			if ready {
				fmt.Fprintln(out, "BaseHarbor control-plane runtime already READY. No changes.")
				return nil
			}
		}
	}

	files, err := bhruntime.ExistingFiles("")
	if err != nil {
		return fmt.Errorf("runtime is not initialized: %w", err)
	}
	cfg, err := bhruntime.LoadConfig(files.Env)
	if err != nil {
		return err
	}
	compose, files, err := startControlPlaneRuntime(ctx, out, bhruntime.Ports{Postgres: cfg.PostgresPort, OpenBao: cfg.OpenBaoPort})
	if err != nil {
		return err
	}
	if err := verifyExistingControlPlaneAfterStart(ctx, compose, files, strings.TrimSpace(recoveryFile), out); err != nil {
		return err
	}
	if err := reconcileControlPlaneServiceAccess(ctx, compose, files); err != nil {
		return fmt.Errorf("reconcile control-plane service access: %w", err)
	}
	if err := resumeSharedPlatformRuntime(ctx, compose, out); err != nil {
		return fmt.Errorf("resume shared platform runtime after verified control plane: %w", err)
	}
	fmt.Fprintln(out, "BaseHarbor control-plane runtime started and ready")
	fmt.Fprintln(out, "next: run 'baha status' and 'baha doctor'")
	return nil
}

func reconcileControlPlaneServiceAccess(ctx context.Context, compose bhruntime.Compose, files bhruntime.Files) error {
	state, err := platformopenbao.Inspect(ctx, compose, files)
	if err != nil {
		return err
	}
	if !state.Initialized || state.Sealed {
		return nil
	}
	if err := platformopenbao.CheckManager(ctx, compose, files); err != nil {
		return err
	}
	issuer := platformopenbao.NewServiceIssuer(compose, files)
	status, err := issuer.Status(ctx)
	if err != nil {
		return err
	}
	if !status.Ready {
		return errors.New("managed service issuer is not ready")
	}
	if err := bhruntime.EnsureServiceAccess(ctx, issuer, files); err != nil {
		return err
	}
	if err := compose.Config(ctx, files.Compose, files.Env); err != nil {
		return err
	}
	return compose.Up(ctx, files.Compose, files.Env)
}

func startControlPlaneRuntime(ctx context.Context, out io.Writer, ports bhruntime.Ports) (bhruntime.Compose, bhruntime.Files, error) {
	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		return bhruntime.Compose{}, bhruntime.Files{}, err
	}
	files, err := bhruntime.EnsureFilesWithPorts("", ports)
	if err != nil {
		return bhruntime.Compose{}, bhruntime.Files{}, err
	}
	if err := compose.Config(ctx, files.Compose, files.Env); err != nil {
		return bhruntime.Compose{}, bhruntime.Files{}, err
	}
	if err := compose.Up(ctx, files.Compose, files.Env); err != nil {
		return bhruntime.Compose{}, bhruntime.Files{}, err
	}
	return compose, files, nil
}

func verifyExistingControlPlaneAfterStart(ctx context.Context, compose bhruntime.Compose, files bhruntime.Files, recoveryFile string, out io.Writer) error {
	var state platformopenbao.State
	var inspectErr error
	deadline := time.Now().Add(30 * time.Second)
	for {
		state, inspectErr = platformopenbao.Inspect(ctx, compose, files)
		if inspectErr == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("verify OpenBao after control-plane start: %w", inspectErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}

	if !state.Initialized {
		fmt.Fprintln(out, "BaseHarbor control-plane runtime started; OpenBao is not initialized yet.")
		fmt.Fprintln(out, "next: run 'baha openbao bootstrap --recovery-file PATH'")
		return nil
	}
	if state.Sealed {
		if recoveryFile == "" {
			return usageError(
				"OpenBao is initialized but sealed after the control-plane restart",
				"Re-run 'baha up --recovery-file /secure/openbao-recovery.json'. BaseHarbor will not unseal OpenBao without operator-held recovery material.",
			)
		}
		fmt.Fprintln(out, "OpenBao is sealed; unsealing from the operator recovery file...")
		if err := platformopenbao.Unseal(ctx, compose, files, recoveryFile); err != nil {
			return fmt.Errorf("unseal OpenBao after control-plane start: %w", err)
		}
	}
	if err := platformopenbao.CheckManager(ctx, compose, files); err != nil {
		return fmt.Errorf("verify OpenBao manager authentication after control-plane start: %w", err)
	}
	if err := reconcileControlPlaneServiceAccess(ctx, compose, files); err != nil {
		return fmt.Errorf("reconcile control-plane service access after start: %w", err)
	}

	var formatted string
	var ok bool
	readinessDeadline := time.Now().Add(30 * time.Second)
	for {
		formatted, ok = health.Format(health.RuntimeChecks())
		if ok {
			fmt.Fprint(out, formatted)
			return nil
		}
		if time.Now().After(readinessDeadline) {
			fmt.Fprint(out, formatted)
			return errors.New("control-plane runtime started but did not become ready")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func runtimeDown(parent context.Context, out io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, time.Minute)
	defer cancel()

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		return err
	}
	files, err := bhruntime.ExistingFiles("")
	if err != nil {
		return fmt.Errorf("runtime is not initialized: %w", err)
	}
	if err := suspendSharedPlatformRuntime(ctx, compose, out); err != nil {
		return fmt.Errorf("suspend shared platform runtime: %w", err)
	}
	if err := compose.Down(ctx, files.Compose, files.Env); err != nil {
		return err
	}
	fmt.Fprintln(out, "BaseHarbor control-plane runtime stopped")
	return nil
}

func suspendSharedPlatformRuntime(ctx context.Context, compose bhruntime.Compose, out io.Writer) error {
	rules, err := application.LoadConnectivityRules()
	if err != nil {
		return err
	}
	if len(rules) > 0 {
		containers, err := compose.ListComposeContainers(ctx)
		if err != nil {
			return err
		}
		for _, rule := range rules {
			if err := suspendConnectivityRule(ctx, compose, rule, containers); err != nil {
				return err
			}
		}
		fmt.Fprintf(out, "[OK] connectivity       suspended %d platform connection(s); policy preserved\n", len(rules))
	}

	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	if files, err := runtimeexecutor.ExistingFiles(dataDir); err == nil {
		if err := compose.StopProject(ctx, runtimeexecutor.ProjectName, files.Compose, files.Env); err != nil {
			return fmt.Errorf("stop shared runtime provider executor: %w", err)
		}
		fmt.Fprintln(out, "[OK] runtime-executor   shared provider executor stopped")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if instances, err := metricsprovider.ExistingSharedProviderInstances(); err != nil {
		return err
	} else {
		for _, instance := range instances {
			if err := compose.StopProject(ctx, instance.Placement.Project, instance.Files.Compose, instance.Files.Env); err != nil {
				return fmt.Errorf("stop shared Prometheus project %s: %w", instance.Placement.Project, err)
			}
		}
		if len(instances) > 0 {
			fmt.Fprintf(out, "[OK] metrics            %d shared Prometheus provider(s) stopped\n", len(instances))
		}
	}
	if files, err := telemetry.ExistingProviderFiles(); err == nil {
		if err := compose.StopProject(ctx, telemetry.ProviderProject, files.Compose, files.Env); err != nil {
			return fmt.Errorf("stop shared telemetry provider: %w", err)
		}
		fmt.Fprintln(out, "[OK] telemetry          shared OpenTelemetry Collector stopped")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if files, err := objectstorage.ExistingProviderFiles(); err == nil {
		if err := compose.StopProject(ctx, objectstorage.ProviderProject, files.Compose, files.Env); err != nil {
			return fmt.Errorf("stop shared object-storage provider: %w", err)
		}
		fmt.Fprintln(out, "[OK] object-storage     shared SeaweedFS provider stopped")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func resumeSharedPlatformRuntime(ctx context.Context, compose bhruntime.Compose, out io.Writer) error {
	if files, err := objectstorage.ExistingProviderFiles(); err == nil {
		if err := compose.ConfigProject(ctx, objectstorage.ProviderProject, files.Compose, files.Env); err != nil {
			return fmt.Errorf("validate shared object-storage provider: %w", err)
		}
		if err := compose.UpProject(ctx, objectstorage.ProviderProject, files.Compose, files.Env); err != nil {
			return fmt.Errorf("start shared object-storage provider: %w", err)
		}
		fmt.Fprintln(out, "[OK] object-storage     shared SeaweedFS provider resumed")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if files, err := telemetry.ExistingProviderFiles(); err == nil {
		if err := compose.ConfigProject(ctx, telemetry.ProviderProject, files.Compose, files.Env); err != nil {
			return fmt.Errorf("validate shared telemetry provider: %w", err)
		}
		if err := compose.UpProject(ctx, telemetry.ProviderProject, files.Compose, files.Env); err != nil {
			return fmt.Errorf("start shared telemetry provider: %w", err)
		}
		fmt.Fprintln(out, "[OK] telemetry          shared OpenTelemetry Collector resumed")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if instances, err := metricsprovider.ExistingSharedProviderInstances(); err != nil {
		return err
	} else {
		for _, instance := range instances {
			if err := compose.ConfigProject(ctx, instance.Placement.Project, instance.Files.Compose, instance.Files.Env); err != nil {
				return fmt.Errorf("validate shared Prometheus project %s: %w", instance.Placement.Project, err)
			}
			if err := compose.UpProject(ctx, instance.Placement.Project, instance.Files.Compose, instance.Files.Env); err != nil {
				return fmt.Errorf("start shared Prometheus project %s: %w", instance.Placement.Project, err)
			}
		}
		if len(instances) > 0 {
			fmt.Fprintf(out, "[OK] metrics            %d shared Prometheus provider(s) resumed\n", len(instances))
		}
	}

	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	if files, err := runtimeexecutor.ExistingFiles(dataDir); err == nil {
		if err := compose.ConfigProject(ctx, runtimeexecutor.ProjectName, files.Compose, files.Env); err != nil {
			return fmt.Errorf("validate shared runtime provider executor: %w", err)
		}
		if err := compose.UpProject(ctx, runtimeexecutor.ProjectName, files.Compose, files.Env); err != nil {
			return fmt.Errorf("start shared runtime provider executor: %w", err)
		}
		fmt.Fprintln(out, "[OK] runtime-executor   shared provider executor resumed")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := reconcileAllConnectivity(ctx, out, compose); err != nil {
		return err
	}
	return nil
}

func reconcileAllConnectivity(ctx context.Context, out io.Writer, compose bhruntime.Compose) error {
	rules, err := application.LoadConnectivityRules()
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return nil
	}
	containers, err := compose.ListComposeContainers(ctx)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		sourceContainers := containersForResolvedEndpoint(rule.Source, containers)
		targetContainers := containersForResolvedEndpoint(rule.Target, containers)
		if len(sourceContainers) == 0 || len(targetContainers) == 0 {
			continue
		}
		targetNetwork, err := resolveConnectivityTargetNetwork(ctx, compose, rule.Target, containers)
		if err != nil {
			return err
		}
		if err := convergeConnectivityRule(ctx, compose, rule, sourceContainers, targetNetwork); err != nil {
			return err
		}
		fmt.Fprintf(out, "[OK] connectivity       %s -> %s\n", formatConnectivityEndpoint(rule.Source), formatConnectivityEndpoint(rule.Target))
	}
	return nil
}

func runtimeDestroy(parent context.Context, args []string, out io.Writer) error {
	confirmed := false
	for _, arg := range args {
		switch arg {
		case "--yes":
			confirmed = true
		default:
			return usageError("unknown argument "+arg, "Usage: baha destroy [--yes]")
		}
	}

	files, err := bhruntime.ExistingFiles("")
	if err != nil {
		return fmt.Errorf("runtime is not initialized: %w", err)
	}
	if err := application.CheckControlPlaneDestroySafe(); err != nil {
		return fmt.Errorf("global destroy preflight: %w", err)
	}
	runtimeDir, err := bhruntime.StateDir("")
	if err != nil {
		return err
	}
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "Global BaseHarbor destroy plan")
	fmt.Fprintln(out, "  control plane: Compose project baseharbor (containers, network and BaseHarbor-owned volumes)")
	if _, err := objectstorage.ExistingProviderFiles(); err == nil {
		fmt.Fprintln(out, "  object storage: shared SeaweedFS provider (container, network and BaseHarbor-owned volume)")
	}
	if _, err := telemetry.ExistingProviderFiles(); err == nil {
		fmt.Fprintln(out, "  telemetry: shared OpenTelemetry Collector provider (container and network)")
	}
	if instances, err := metricsprovider.ExistingSharedProviderInstances(); err == nil && len(instances) > 0 {
		fmt.Fprintf(out, "  metrics: %d shared Prometheus provider instance(s) across default/sharing boundaries\n", len(instances))
	}
	fmt.Fprintf(out, "  runtime state: %s\n", runtimeDir)
	fmt.Fprintf(out, "  registry:      %s\n", filepath.Join(dataDir, "provider-registry.json"))
	if records, trustErr := hosttrust.StateRecords(dataDir); trustErr != nil {
		return fmt.Errorf("inspect BaseHarbor-owned host trust: %w", trustErr)
	} else if len(records) > 0 {
		fmt.Fprintf(out, "  host trust:    %d BaseHarbor-owned CA anchor(s)\n", len(records))
	}
	fmt.Fprintln(out, "  application-owned repository data/volumes: preserved")
	if !confirmed {
		fmt.Fprintln(out, "No changes were made. Re-run with --yes to permanently remove the global BaseHarbor control plane.")
		return nil
	}

	ctx, cancel := context.WithTimeout(parent, time.Minute)
	defer cancel()
	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		return err
	}
	if removed, err := hosttrust.RemoveOwned(ctx, dataDir); err != nil {
		return fmt.Errorf("remove BaseHarbor-owned host trust before destroy: %w", err)
	} else if removed > 0 {
		fmt.Fprintf(out, "[OK] host trust         removed %d BaseHarbor-owned CA anchor(s)\n", removed)
	}
	if relays, err := connectivityrelay.ExistingInstances(); err != nil {
		return fmt.Errorf("inspect connectivity relay state: %w", err)
	} else {
		for _, relay := range relays {
			if err := compose.DestroyProject(ctx, relay.Project, relay.Compose, relay.Env); err != nil {
				return fmt.Errorf("destroy connectivity relay project %s: %w", relay.Project, err)
			}
		}
	}
	if err := runtimeexecutor.DestroyShared(ctx, compose, dataDir); err != nil {
		return fmt.Errorf("destroy shared runtime provider executor: %w", err)
	}
	if err := objectstorage.DestroySharedProvider(ctx, compose); err != nil {
		return fmt.Errorf("destroy shared object-storage provider: %w", err)
	}
	if err := telemetry.DestroySharedProvider(ctx, compose); err != nil {
		return fmt.Errorf("destroy shared telemetry provider: %w", err)
	}
	if err := tracesprovider.DestroyAllSharedProviders(ctx, compose); err != nil {
		return fmt.Errorf("destroy shared traces providers: %w", err)
	}
	if err := metricsprovider.DestroyAllSharedProviders(ctx, compose); err != nil {
		return fmt.Errorf("destroy shared metrics providers: %w", err)
	}
	if err := compose.DestroyProject(ctx, "baseharbor", files.Compose, files.Env); err != nil {
		return fmt.Errorf("destroy BaseHarbor control-plane Compose project: %w", err)
	}
	if err := os.RemoveAll(runtimeDir); err != nil {
		return fmt.Errorf("remove BaseHarbor runtime state: %w", err)
	}
	for _, name := range []string{
		"provider-registry.json",
		"provider-registry.json.lock",
		"connectivity.json",
		"connectivity.json.lock",
	} {
		path := filepath.Join(dataDir, name)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove BaseHarbor platform state %s: %w", name, err)
		}
	}
	if err := os.RemoveAll(filepath.Join(dataDir, "connectivity")); err != nil {
		return fmt.Errorf("remove BaseHarbor connectivity runtime state: %w", err)
	}
	fmt.Fprintln(out, "BaseHarbor global control plane was permanently destroyed.")
	return nil
}

func runtimeStatus(parent context.Context, out io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		return err
	}
	files, err := bhruntime.ExistingFiles("")
	if err != nil {
		return fmt.Errorf("runtime is not initialized: %w", err)
	}
	status, err := compose.Status(ctx, files.Compose, files.Env)
	if err != nil {
		return err
	}
	fmt.Fprint(out, status)
	running, err := compose.RunningServicesProject(ctx, "baseharbor", files.Compose, files.Env)
	if err != nil {
		return err
	}
	if len(running) == 0 {
		fmt.Fprintln(out, "BaseHarbor control-plane runtime is stopped.")
		return nil
	}

	checks := health.RuntimeChecks()
	if len(checks) == 0 {
		return nil
	}
	formatted, ok := health.Format(checks)
	fmt.Fprint(out, formatted)
	if !ok {
		return errors.New("runtime is running but not ready")
	}
	return nil
}

func initConfig(out io.Writer) error {
	const path = config.DefaultFile
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	cfg := config.Default()
	if err := os.WriteFile(path, []byte(cfg.YAML()), 0o600); err != nil {
		return err
	}
	fmt.Fprintf(out, "created %s\n", path)
	fmt.Fprintln(out, "next: run 'baha doctor'")
	return nil
}
