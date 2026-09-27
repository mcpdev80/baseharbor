package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
)

func devCommand() *cli.Command {
	return &cli.Command{
		Name:    "dev",
		Summary: "Manage local development conveniences",
		Usage:   "baha dev <credentials|domain>",
		Children: []*cli.Command{
			{
				Name:    "credentials",
				Summary: "Show or rotate the target-scoped development management login",
				Usage:   "baha dev credentials [--reset] [--username USER] [--password-file FILE]",
				Long:    "Shows the explicit local development management login for the effective Target. The secret is revealed only by this command and is never included in status, plan, doctor, evidence or application manifests. --reset generates a new strong password. --password-file installs an explicit password from an owner-only file.",
				Run:     devCredentialsCommand,
			},
			{
				Name:    "domain",
				Summary: "Show or configure the target-scoped development domain",
				Usage:   "baha dev domain [DOMAIN]",
				Long:    "Shows the development domain used to derive canonical local application and management URLs. Supplying DOMAIN updates the target-wide value; application manifests are not modified.",
				Run:     devDomainCommand,
			},
		},
	}
}

func devDomainCommand(ctx context.Context, args []string, out, errOut io.Writer) error {
	if len(args) > 1 {
		return usageError("baha dev domain accepts at most one domain", "Run 'baha dev domain --help' for usage.")
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	var domain string
	if len(args) == 1 {
		domain, err = devaccess.ConfigureDomain(target.Name, args[0])
	} else {
		domain, err = devaccess.EnsureDomain(target.Name)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Development domain\n")
	fmt.Fprintf(out, "  Target  %s\n", target.Name)
	fmt.Fprintf(out, "  Domain  %s\n", domain)
	fmt.Fprintln(out, "  Scope   target / dev")
	if len(args) == 1 {
		fmt.Fprintln(out, "Run 'baha up' to reconcile canonical development routes.")
	}
	return nil
}

func devCredentialsCommand(ctx context.Context, args []string, out, errOut io.Writer) error {
	reset := false
	username := ""
	passwordFile := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--reset":
			reset = true
		case "--username", "--password-file":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return usageError(args[i]+" requires a value", "Run 'baha dev credentials --help' for usage.")
			}
			value := strings.TrimSpace(args[i+1])
			i++
			if args[i-1] == "--username" {
				username = value
			} else {
				passwordFile = value
			}
		default:
			return unknownOptionUsage("baha dev credentials", args[i], "--reset", "--username", "--password-file")
		}
	}

	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	const environment = "dev"

	var credentials devaccess.Credentials
	switch {
	case passwordFile != "":
		password, err := readDevAccessPasswordFile(passwordFile)
		if err != nil {
			return err
		}
		credentials, err = devaccess.Configure(target.Name, environment, username, password)
		if err != nil {
			return err
		}
	case reset || username != "":
		credentials, err = devaccess.Reset(target.Name, environment, username)
		if err != nil {
			return err
		}
	default:
		credentials, err = devaccess.Ensure(target.Name, environment)
		if err != nil {
			return err
		}
	}

	fmt.Fprintf(out, "Developer management access\n")
	fmt.Fprintf(out, "  Target    %s\n", target.Name)
	fmt.Fprintf(out, "  Scope     dev\n")
	fmt.Fprintf(out, "  Username  %s\n", credentials.Username)
	fmt.Fprintf(out, "  Password  %s\n", credentials.Password)
	fmt.Fprintln(out, "\nThis explicit command reveals the secret. Normal BaseHarbor status, plans and evidence never do.")
	if reset || passwordFile != "" || username != "" {
		fmt.Fprintln(out, "Run 'baha up' to reconcile the updated login into selected development management UIs.")
	}
	return nil
}

func readDevAccessPasswordFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", errors.New("developer access password file must be a regular non-symlink file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("developer access password file is accessible by group or others (%o)", info.Mode().Perm())
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(io.LimitReader(file, 64<<10))
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", errors.New("developer access password file is empty")
	}
	password := scanner.Text()
	if strings.TrimSpace(password) == "" || strings.ContainsAny(password, "\r\n") {
		return "", errors.New("developer access password is invalid")
	}
	if scanner.Scan() {
		return "", errors.New("developer access password file must contain exactly one line")
	}
	return password, scanner.Err()
}
