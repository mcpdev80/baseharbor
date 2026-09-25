package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
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
	if _, err := existingTargetRuntimeFiles(parent); err == nil {
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
