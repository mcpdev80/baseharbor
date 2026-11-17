package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/cli"
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
			fmt.Fprintln(out, "OpenBao trust plane")
			// Every phase line is printed before the exit-code signal, so a
			// failed state shows the complete plane picture instead of
			// stopping at the first failing phase.
			failure := error(nil)
			fmt.Fprintf(out, "%s initialized        %s\n", report.StatusMarker(report.Initialized), report.Answer(report.Initialized))
			if !report.Initialized {
				failure = errors.New("OpenBao is not initialized")
			}
			fmt.Fprintf(out, "%s unsealed           %s\n", report.StatusMarker(report.Unsealed), report.Answer(report.Unsealed))
			if report.Initialized && !report.Unsealed {
				failure = errors.New("OpenBao is sealed")
			}
			fmt.Fprintf(out, "%s manager auth       ", report.StatusMarker(report.ManagerReady))
			if report.ManagerReady {
				fmt.Fprintln(out, "AppRole login succeeded")
			} else {
				fmt.Fprintln(out, "unavailable")
				if failure == nil && report.Initialized {
					failure = errors.New("OpenBao manager authentication is not ready")
				}
			}
			return failure
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
			if format == outputJSON {
				if err := bootstrapManagedOpenBao(bootstrapCtx, recoveryPath); err != nil {
					return err
				}
			} else if _, isProgressWriter := out.(interface{ ActivityDetail(string) }); isProgressWriter {
				if err := bootstrapManagedOpenBaoProgress(bootstrapCtx, recoveryPath, func(detail string) {
					cli.ReportActivityDetail(out, detail)
				}); err != nil {
					return err
				}
			} else {
				term := cli.NewTerminal(ctx, out, errOut)
				if err := progress(ctx, term, "Initialize and secure OpenBao", func(progressOut io.Writer) error {
					return bootstrapManagedOpenBaoProgress(bootstrapCtx, recoveryPath, func(detail string) {
						cli.ReportActivityDetail(progressOut, detail)
					})
				}); err != nil {
					return err
				}
			}
			if format == outputJSON {
				report, err := inspectManagedOpenBao(bootstrapCtx)
				if err != nil {
					return err
				}
				return writeJSON(out, report)
			}
			cli.ReportActivityNotice(out, "[OK] OpenBao initialized and unsealed")
			cli.ReportActivityNotice(out, "[OK] baseharbor KV v2 mount configured")
			cli.ReportActivityNotice(out, "[OK] restricted manager AppRole configured and verified")
			cli.ReportActivityNotice(out, "[OK] initial root token revoked")
			cli.ReportActivityNotice(out, fmt.Sprintf("Recovery file: %s", recoveryPath))
			cli.ReportActivityNotice(out, "Store the recovery file securely and separately from BaseHarbor application state.")
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

// openBaoRuntime resolves the runtime for the effective target. It is a package
// seam so tests can supply a minimal initialized runtime for the reconcile path.
var openBaoRuntime = func(ctx context.Context) (bhruntime.RuntimeProvider, bhruntime.Files, error) {
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
