package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
)

type guidedDevAccessSetup struct {
	target               string
	domain               string
	username             string
	password             []byte
	configureDomain      bool
	configureCredentials bool
	generatePassword     bool
}

func collectGuidedDevAccess(ctx context.Context, reader *bufio.Reader, out io.Writer, m application.Manifest) (guidedDevAccessSetup, error) {
	var setup guidedDevAccessSetup
	if !devaccess.Enabled(m.Environment) || !requiresDevelopmentGateway(m) {
		return setup, nil
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return setup, err
	}
	setup.target = target.Name

	domain, err := devaccess.LoadDomain(target.Name)
	switch {
	case err == nil:
		setup.domain = domain
	case errors.Is(err, os.ErrNotExist):
		defaultDomain := devaccess.DefaultDomain
		if target.AccessProvider != "" && target.AccessProvider != "local" {
			defaultDomain, err = remoteDevelopmentDomain(ctx, target, "")
			if err != nil {
				return setup, err
			}
		}
		value, promptErr := promptLine(reader, out, "Development domain", defaultDomain)
		if promptErr != nil {
			return setup, promptErr
		}
		setup.domain = value
		setup.configureDomain = true
	default:
		return setup, err
	}

	_, err = devaccess.Load(target.Name, "dev")
	switch {
	case err == nil:
		return setup, nil
	case !errors.Is(err, os.ErrNotExist):
		return setup, err
	}

	fmt.Fprintln(out, "\nDeveloper admin access")
	username, err := promptLine(reader, out, "Username", devaccess.DefaultUsername)
	if err != nil {
		return setup, err
	}
	setup.username = username
	setup.configureCredentials = true

	useGenerated, err := promptYesNo(reader, out, "Use a securely generated password?", true)
	if err != nil {
		return setup, err
	}
	if useGenerated {
		setup.generatePassword = true
		return setup, nil
	}
	password, err := readApplicationSecretFromTerminal(appInitInput, out, "Developer admin password")
	if err != nil {
		return setup, err
	}
	setup.password = password
	return setup, nil
}

func printGuidedDevAccessSummary(out io.Writer, setup guidedDevAccessSetup) {
	if setup.target == "" {
		return
	}
	fmt.Fprintln(out, "\nDevelopment access")
	fmt.Fprintf(out, "  Target        %s\n", setup.target)
	fmt.Fprintf(out, "  Domain        %s\n", setup.domain)
	if setup.configureCredentials {
		fmt.Fprintf(out, "  Username      %s\n", setup.username)
		if setup.generatePassword {
			fmt.Fprintln(out, "  Password      securely generated")
		} else {
			fmt.Fprintln(out, "  Password      custom (hidden)")
		}
		fmt.Fprintln(out, "  Reveal later  baha dev credentials")
	} else {
		fmt.Fprintln(out, "  Dev login     existing target credentials")
	}
}

var guidedRemoteCredentialReconcile = reconcileRemoteDevelopmentCredentials
var guidedDevelopmentDomainConfigure = configureDevelopmentDomain

func applyGuidedDevAccess(ctx context.Context, setup guidedDevAccessSetup) error {
	defer zeroBytes(setup.password)
	if setup.target == "" {
		return nil
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	if target.Name != setup.target {
		return errors.New("development access Target changed; credentials were not applied")
	}
	if setup.configureDomain {
		if _, err := guidedDevelopmentDomainConfigure(ctx, setup.domain); err != nil {
			return err
		}
	}
	if !setup.configureCredentials {
		return nil
	}
	previous, previousErr := devaccess.Load(setup.target, "dev")
	if previousErr != nil && !errors.Is(previousErr, os.ErrNotExist) {
		return previousErr
	}
	var credentials devaccess.Credentials
	if setup.generatePassword {
		credentials, err = devaccess.Reset(setup.target, "dev", setup.username)
	} else {
		credentials, err = devaccess.Configure(setup.target, "dev", setup.username, string(setup.password))
	}
	if err != nil {
		return err
	}
	if target.AccessProvider != "" && target.AccessProvider != "local" {
		// Choosing a login in the wizard is explicit credential configuration,
		// including replacement of an existing default on the selected Core.
		if err := guidedRemoteCredentialReconcile(ctx, target, credentials.Username, credentials.Password, true); err != nil {
			if previousErr == nil {
				_, _ = devaccess.Configure(setup.target, "dev", previous.Username, previous.Password)
			} else if path, pathErr := devaccess.Path(setup.target, "dev"); pathErr == nil {
				_ = os.Remove(path)
			}
			return fmt.Errorf("configure selected remote Core development access: %w", err)
		}
	}
	return nil
}
