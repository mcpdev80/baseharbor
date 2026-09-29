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
		value, promptErr := promptLine(reader, out, "Development domain", devaccess.DefaultDomain)
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
	fmt.Fprintln(out, "\nLocal development")
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

func applyGuidedDevAccess(setup guidedDevAccessSetup) error {
	if setup.target == "" {
		return nil
	}
	if setup.configureDomain {
		if _, err := devaccess.ConfigureDomain(setup.target, setup.domain); err != nil {
			return err
		}
	}
	if !setup.configureCredentials {
		return nil
	}
	if setup.generatePassword {
		_, err := devaccess.Reset(setup.target, "dev", setup.username)
		return err
	}
	defer zeroBytes(setup.password)
	_, err := devaccess.Configure(setup.target, "dev", setup.username, string(setup.password))
	return err
}
