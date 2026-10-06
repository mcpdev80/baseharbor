package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/cli"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func openBaoCommand() *cli.Command {
	command := &cli.Command{
		Name:    "openbao",
		Summary: "Bootstrap and operate the BaseHarbor OpenBao trust plane",
		Usage:   "baha openbao <command> [options]",
		Long:    "Manages the bundled OpenBao HA trust plane. Bootstrap, unseal and rotation are explicit security-sensitive operations; secret material is never printed by these commands.",
	}
	command.Children = []*cli.Command{
		openBaoStatusCommand(),
		openBaoBootstrapCommand(),
		openBaoUnsealCommand(),
		openBaoRotateCommand(),
	}
	return command
}

func openBaoStatusCommand() *cli.Command {
	return &cli.Command{
		Name:    "status",
		Summary: "Show OpenBao initialization, seal and manager-auth state",
		Usage:   "baha openbao status",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "openbao status")
			if err != nil {
				return err
			}
			args = filtered
			if len(args) != 0 {
				return usageError("baha openbao status does not accept arguments", "Run 'baha openbao status --help' for usage.")
			}
			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			report, err := inspectManagedOpenBao(checkCtx)
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, report)
			}
			state := platformopenbao.State{Initialized: report.Initialized, Sealed: !report.Unsealed}
			fmt.Fprintln(out, "OpenBao trust plane")
			if !state.Initialized {
				fmt.Fprintln(out, "[FAIL] initialized        no")
				return errors.New("OpenBao is not initialized")
			}
			fmt.Fprintln(out, "[OK] initialized        yes")
			if state.Sealed {
				fmt.Fprintln(out, "[FAIL] unsealed           no")
				return errors.New("OpenBao is sealed")
			}
			fmt.Fprintln(out, "[OK] unsealed           yes")
			if !report.ManagerReady {
				fmt.Fprintln(out, "[FAIL] manager auth       unavailable")
				return errors.New("OpenBao manager authentication is not ready")
			}
			fmt.Fprintln(out, "[OK] manager auth       AppRole login succeeded")
			return nil
		},
	}
}

func openBaoBootstrapCommand() *cli.Command {
	return &cli.Command{
		Name:    "bootstrap",
		Summary: "Initialize OpenBao and establish the BaseHarbor manager identity",
		Usage:   "baha openbao bootstrap --recovery-file PATH",
		Long:    "Initializes the bundled single-node OpenBao instance with one Shamir key share, stores only the unseal material in the explicitly selected owner-only recovery file, configures the baseharbor KV v2 mount and a restricted manager AppRole, verifies that identity, and revokes the initial root token. The recovery file must be outside .baseharbor state.\n\nOptions:\n  --recovery-file PATH  Required destination for OpenBao unseal recovery material",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "openbao bootstrap")
			if err != nil {
				return err
			}
			args = filtered
			recoveryPath, err := parseRecoveryFileArg("bootstrap", args)
			if err != nil {
				return err
			}
			bootstrapCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			if err := bootstrapManagedOpenBao(bootstrapCtx, recoveryPath); err != nil {
				return err
			}
			if format == outputJSON {
				report, err := inspectManagedOpenBao(bootstrapCtx)
				if err != nil {
					return err
				}
				return writeJSON(out, report)
			}
			fmt.Fprintln(out, "[OK] OpenBao initialized and unsealed")
			fmt.Fprintln(out, "[OK] baseharbor KV v2 mount configured")
			fmt.Fprintln(out, "[OK] restricted manager AppRole configured and verified")
			fmt.Fprintln(out, "[OK] initial root token revoked")
			fmt.Fprintf(out, "Recovery file: %s\n", recoveryPath)
			fmt.Fprintln(out, "Store the recovery file securely and separately from BaseHarbor application state.")
			return nil
		},
	}
}

func openBaoUnsealCommand() *cli.Command {
	return &cli.Command{
		Name:    "unseal",
		Summary: "Unseal OpenBao from an operator-held recovery file",
		Usage:   "baha openbao unseal --recovery-file PATH",
		Long:    "Reads owner-only recovery material from the explicitly supplied file and sends the unseal key to OpenBao over stdin. The key is never printed or placed in the host command argument list.\n\nOptions:\n  --recovery-file PATH  Required owner-only OpenBao recovery file",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "openbao unseal")
			if err != nil {
				return err
			}
			args = filtered
			recoveryPath, err := parseRecoveryFileArg("unseal", args)
			if err != nil {
				return err
			}
			unsealCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			if err := unsealManagedOpenBao(unsealCtx, recoveryPath); err != nil {
				return err
			}
			if format == outputJSON {
				report, err := inspectManagedOpenBao(unsealCtx)
				if err != nil {
					return err
				}
				return writeJSON(out, report)
			}
			fmt.Fprintln(out, "[OK] OpenBao is unsealed")
			fmt.Fprintln(out, "[OK] manager AppRole authentication succeeded")
			return nil
		},
	}
}

func openBaoRotateCommand() *cli.Command {
	return &cli.Command{
		Name:    "rotate",
		Summary: "Rotate OpenBao/control-plane credentials and managed service PKI",
		Usage:   "baha openbao rotate --recovery-file PATH",
		Long:    "Rotates the restricted OpenBao manager AppRole credential, PostgreSQL control-plane administration/replication/OpenBao-storage credentials, and the managed service CA. Replacement credentials and trust are verified before previous material is retired. Secret values are never printed.\n\nOptions:\n  --recovery-file PATH  Required owner-only OpenBao recovery file used if a rolling member restart requires unseal",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "openbao rotate")
			if err != nil {
				return err
			}
			args = filtered
			recoveryPath, err := parseRecoveryFileArg("rotate", args)
			if err != nil {
				return err
			}
			rotateCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
			defer cancel()

			if err := rotateManagedOpenBao(rotateCtx, recoveryPath); err != nil {
				return err
			}
			if format == outputJSON {
				report, err := inspectManagedOpenBao(rotateCtx)
				if err != nil {
					return err
				}
				return writeJSON(out, report)
			}
			fmt.Fprintln(out, "[OK] control-plane database credentials rotated and previous logins retired")
			fmt.Fprintln(out, "[OK] OpenBao manager AppRole credential rotated and previous SecretID retired")
			fmt.Fprintln(out, "[OK] managed service certificates/CA rotated and previous CA retired")
			fmt.Fprintln(out, "[OK] OpenBao API/UI and PostgreSQL stable endpoints verified")
			return nil
		},
	}
}

func openBaoRuntime(ctx context.Context) (bhruntime.RuntimeProvider, bhruntime.Files, error) {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return nil, bhruntime.Files{}, err
	}
	compose, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return nil, bhruntime.Files{}, err
	}
	files, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		return nil, bhruntime.Files{}, fmt.Errorf("BaseHarbor runtime is not initialized: %w", err)
	}
	return compose, files, nil
}

func parseRecoveryFileArg(command string, args []string) (string, error) {
	var path string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--recovery-file":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", usageError("--recovery-file requires PATH", "Example: baha openbao "+command+" --recovery-file /secure/openbao-recovery.json")
			}
			i++
			path = args[i]
		case strings.HasPrefix(arg, "--recovery-file="):
			path = strings.TrimPrefix(arg, "--recovery-file=")
		default:
			return "", usageError("unknown argument "+arg, "Run 'baha openbao "+command+" --help' for usage.")
		}
	}
	if strings.TrimSpace(path) == "" {
		return "", usageError("baha openbao "+command+" requires --recovery-file PATH", "The recovery file must be stored outside .baseharbor state.")
	}
	return path, nil
}
