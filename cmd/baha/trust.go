package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/hosttrust"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func trustCommand() *cli.Command {
	cmd := &cli.Command{
		Name:    "trust",
		Summary: "Inspect, export or explicitly install the managed local BaseHarbor CA",
		Usage:   "baha trust <status|export|install> [options]",
		Long:    "Operates only on public trust material for the managed-local issuer. Private CA keys remain inside the issuer provider. External PKI and BYOC trust roots are never claimed or installed as BaseHarbor-owned host trust.",
	}
	cmd.Children = []*cli.Command{
		trustStatusCommand(),
		trustExportCommand(),
		trustInstallCommand(),
	}
	return cmd
}

func trustStatusCommand() *cli.Command {
	return &cli.Command{
		Name:    "status",
		Summary: "Show whether the managed-local CA is trusted by this host",
		Usage:   "baha trust status",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 0 {
				return usageError("baha trust status does not accept arguments", "Run 'baha trust status --help' for usage.")
			}
			bundle, managed, err := currentManagedTrustBundle(ctx)
			if err != nil {
				return err
			}
			if !managed {
				fmt.Fprintln(out, "Host trust is operator-owned because the active OpenBao service-access PKI source is not managed-local.")
				return nil
			}
			dataDir, err := bhruntime.DataDir("")
			if err != nil {
				return err
			}
			status, err := hosttrust.Inspect(dataDir, bundle.PEM)
			if err != nil {
				return err
			}
			fmt.Fprintln(out, "BaseHarbor managed-local host trust")
			fmt.Fprintf(out, "CA fingerprint: %s\n", status.Fingerprint)
			if status.Trusted {
				fmt.Fprintln(out, "[OK] host trust         CA is trusted")
			} else {
				fmt.Fprintln(out, "[WARN] host trust       CA is not trusted")
			}
			if status.Owned {
				fmt.Fprintf(out, "[OK] ownership          BaseHarbor (%s)\n", status.Backend)
			} else {
				fmt.Fprintln(out, "[INFO] ownership        not BaseHarbor-owned")
			}
			return nil
		},
	}
}

func trustExportCommand() *cli.Command {
	return &cli.Command{
		Name:    "export",
		Summary: "Export only the public managed-local CA certificate",
		Usage:   "baha trust export --output PATH",
		Long:    "Exports the public CA certificate/bundle for another developer machine or client trust store. Private CA keys and issuer state are never exported.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			path, err := parseTrustOutputArg(args)
			if err != nil {
				return err
			}
			bundle, managed, err := currentManagedTrustBundle(ctx)
			if err != nil {
				return err
			}
			if !managed {
				return errors.New("the active OpenBao service-access PKI source is external-pki/BYOC; BaseHarbor does not export or claim ownership of that trust root")
			}
			if err := hosttrust.Export(path, bundle.PEM); err != nil {
				return err
			}
			fmt.Fprintf(out, "Exported public BaseHarbor CA: %s\n", path)
			return nil
		},
	}
}

func trustInstallCommand() *cli.Command {
	return &cli.Command{
		Name:    "install",
		Summary: "Explicitly install the managed-local CA into the host trust store",
		Usage:   "baha trust install --yes",
		Long:    "Installs only the public managed-local CA and records BaseHarbor ownership. This is an explicit host mutation and therefore requires --yes even when invoked directly.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			confirmed := false
			for _, arg := range args {
				switch arg {
				case "--yes", "-y":
					confirmed = true
				default:
					return usageError("unknown argument "+arg, "Usage: baha trust install --yes")
				}
			}
			if !confirmed {
				return usageError("baha trust install requires explicit --yes consent", "Re-run 'baha trust install --yes' to modify the host trust store.")
			}
			bundle, managed, err := currentManagedTrustBundle(ctx)
			if err != nil {
				return err
			}
			if !managed {
				return errors.New("the active OpenBao service-access PKI source is external-pki/BYOC; BaseHarbor will not install or claim that trust root")
			}
			dataDir, err := bhruntime.DataDir("")
			if err != nil {
				return err
			}
			status, err := hosttrust.Install(ctx, dataDir, bundle.PEM, bundle.IssuerReference, nil)
			if err != nil {
				return err
			}
			if status.Owned {
				fmt.Fprintf(out, "[OK] host trust         installed BaseHarbor-managed CA via %s\n", status.Backend)
			} else {
				fmt.Fprintln(out, "[OK] host trust         CA was already trusted; ownership remains external")
			}
			return nil
		},
	}
}

func parseTrustOutputArg(args []string) (string, error) {
	var path string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--output", args[i] == "-o":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", usageError("--output requires PATH", "Example: baha trust export --output ./baseharbor-ca.pem")
			}
			i++
			path = strings.TrimSpace(args[i])
		case strings.HasPrefix(args[i], "--output="):
			path = strings.TrimSpace(strings.TrimPrefix(args[i], "--output="))
		default:
			return "", usageError("unknown argument "+args[i], "Usage: baha trust export --output PATH")
		}
	}
	if path == "" {
		return "", usageError("baha trust export requires --output PATH", "Example: baha trust export --output ./baseharbor-ca.pem")
	}
	return path, nil
}

func currentManagedTrustBundle(ctx context.Context) (serviceaccess.TrustBundle, bool, error) {
	policy, err := serviceaccess.Resolve("prod", "openbao", serviceaccess.AuthenticationNative)
	if err != nil {
		return serviceaccess.TrustBundle{}, false, err
	}
	if policy.PKISource != serviceaccess.PKIManagedLocal {
		return serviceaccess.TrustBundle{}, false, nil
	}

	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	compose, files, err := openBaoRuntime(checkCtx)
	if err != nil {
		return serviceaccess.TrustBundle{}, true, err
	}
	state, err := platformopenbao.Inspect(checkCtx, compose, files)
	if err != nil {
		return serviceaccess.TrustBundle{}, true, err
	}
	if !state.Initialized || state.Sealed {
		return serviceaccess.TrustBundle{}, true, errors.New("managed-local CA is unavailable until OpenBao is initialized and unsealed")
	}
	if err := platformopenbao.CheckManager(checkCtx, compose, files); err != nil {
		return serviceaccess.TrustBundle{}, true, errors.New("managed-local CA is unavailable because OpenBao manager authentication is not ready")
	}
	issuer := platformopenbao.NewServiceIssuer(compose, files)
	bundle, err := issuer.TrustBundle(checkCtx)
	if err != nil {
		return serviceaccess.TrustBundle{}, true, err
	}
	return bundle, true, nil
}

func maybeOfferManagedHostTrust(ctx context.Context, in io.Reader, out io.Writer, opts runtimeUpOptions) error {
	bundle, managed, err := currentManagedTrustBundle(ctx)
	if err != nil {
		return err
	}
	if !managed {
		return nil
	}
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	status, err := hosttrust.Inspect(dataDir, bundle.PEM)
	if err != nil {
		return err
	}
	if status.Trusted {
		return nil
	}
	if opts.TrustHostCA {
		_, err := hosttrust.Install(ctx, dataDir, bundle.PEM, bundle.IssuerReference, nil)
		if err != nil {
			return fmt.Errorf("install managed-local CA into host trust: %w", err)
		}
		fmt.Fprintln(out, "[OK] host trust         managed-local CA installed after explicit opt-in")
		return nil
	}

	if opts.Yes || noInput(ctx) || !readerIsTerminal(in) {
		fmt.Fprintln(out, "[INFO] host trust       managed-local CA is not trusted by this host")
		fmt.Fprintln(out, "       To opt in explicitly: baha trust install --yes")
		return nil
	}

	reader := bufio.NewReader(in)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "BaseHarbor uses a managed local CA for HTTPS/TLS endpoints.")
	fmt.Fprintln(out, "Trusting it on this host avoids per-tool CA configuration.")
	fmt.Fprint(out, "Install the BaseHarbor CA into the host trust store? [y/N]: ")
	answer, readErr := reader.ReadString('\n')
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" && answer != "j" && answer != "ja" {
		fmt.Fprintln(out, "Host trust unchanged. Public CA export remains available with 'baha trust export --output PATH'.")
		return nil
	}
	_, err = hosttrust.Install(ctx, dataDir, bundle.PEM, bundle.IssuerReference, nil)
	if err != nil {
		return fmt.Errorf("install managed-local CA into host trust: %w", err)
	}
	fmt.Fprintln(out, "[OK] host trust         managed-local CA installed")
	return nil
}
