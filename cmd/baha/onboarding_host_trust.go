package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/hosttrust"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type onboardTrustOptions struct{ install, skip bool }

func stripOnboardTrustOptions(args []string) ([]string, onboardTrustOptions, error) {
	var options onboardTrustOptions
	var rest []string
	for _, arg := range args {
		switch arg {
		case "--trust-host-ca":
			options.install = true
		case "--skip-host-trust":
			options.skip = true
		default:
			rest = append(rest, arg)
		}
	}
	if options.install && options.skip {
		return nil, options, usageError("conflicting CA trust options", "Use either --trust-host-ca or --skip-host-trust.")
	}
	return rest, options, nil
}

var installImportedPublicTrust = installWorkstationPublicTrust

func offerImportedHostTrust(ctx context.Context, reader *bufio.Reader, out io.Writer, target string, publicCA []byte, options onboardTrustOptions) error {
	retry := "baha --target " + target + " trust install --yes"
	if options.skip || (!options.install && (noInput(ctx) || !readerIsTerminal(onboardingInput))) {
		fmt.Fprintln(out, "System/browser CA trust unchanged. To enable HTTPS without certificate warnings: "+retry)
		return nil
	}
	if !options.install {
		_, fingerprint, err := hosttrust.ParseCA(publicCA)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, "\nTrust this Core's public CA for application and management HTTPS on this workstation?")
		fmt.Fprintln(out, "This installs a CA into system trust (sudo may be needed) and supported existing browser certificate stores.")
		fmt.Fprintln(out, "Certificates signed by this CA will be trusted; no private keys are imported.")
		fmt.Fprintln(out, "CA SHA-256: "+fingerprint)
		fmt.Fprint(out, "Install system and browser CA trust? [y/N]: ")
		answer, _ := reader.ReadString('\n')
		if answer = strings.ToLower(strings.TrimSpace(answer)); answer != "y" && answer != "yes" {
			fmt.Fprintln(out, "Connection saved; CA trust unchanged. Later: "+retry)
			return nil
		}
	}
	stateDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	status, browsers, err := installImportedPublicTrust(ctx, stateDir, publicCA, "core-target/"+target)
	if err != nil {
		return fmt.Errorf("connection saved; CA trust installation failed (do not import the invitation again); retry %s: %w", retry, err)
	}
	if !status.Trusted {
		return fmt.Errorf("connection saved, but system CA trust could not be verified; retry %s", retry)
	}
	fmt.Fprintln(out, "[OK] System CA trust installed.")
	renderBrowserTrust(out, browsers)
	return nil
}

func installWorkstationPublicTrust(ctx context.Context, stateDir string, publicCA []byte, issuer string) (hosttrust.Status, []hosttrust.BrowserStatus, error) {
	status, err := hosttrust.Install(ctx, stateDir, publicCA, issuer, nil)
	if err != nil {
		return status, nil, err
	}
	browsers, err := hosttrust.InstallBrowsers(ctx, stateDir, publicCA)
	return status, browsers, err
}

func renderBrowserTrust(out io.Writer, browsers []hosttrust.BrowserStatus) {
	for _, browser := range browsers {
		if browser.State == "trusted" {
			fmt.Fprintf(out, "[OK] Browser CA trust: %s (restart the browser).\n", browser.Database)
		} else {
			fmt.Fprintf(out, "[INFO] Browser CA trust: %s\n", browser.Detail)
		}
	}
}
