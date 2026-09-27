package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

func operatorLoginCommand() *cli.Command {
	return &cli.Command{
		Name:    "login",
		Summary: "Authenticate the local BaseHarbor operator through OIDC",
		Usage:   "baha login",
		Long:    "Starts the standard OIDC Authorization Code + PKCE flow and stores only a short-lived owner-only local operator session. BaseHarbor does not maintain a separate username/password database.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 0 {
				return usageError("baha login does not accept arguments", "Configure the operator OIDC issuer/client through deployment policy, then run 'baha login'.")
			}
			cfg, err := operatorauth.ConfigFromEnv()
			if err != nil {
				return err
			}
			principal, err := operatorauth.Login(ctx, cfg, out)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Authenticated operator: %s\nIssuer: %s\n", principal.Subject, principal.Issuer)
			return nil
		},
	}
}

func operatorLogoutCommand() *cli.Command {
	return &cli.Command{
		Name:    "logout",
		Summary: "Remove the local BaseHarbor operator session",
		Usage:   "baha logout",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 0 {
				return usageError("baha logout does not accept arguments", "Run 'baha logout'.")
			}
			if err := operatorauth.ClearSession(); err != nil {
				return err
			}
			fmt.Fprintln(out, "Operator session removed.")
			return nil
		},
	}
}

func operatorWhoAmICommand() *cli.Command {
	return &cli.Command{
		Name:    "whoami",
		Summary: "Show the authenticated BaseHarbor operator identity",
		Usage:   "baha whoami",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 0 {
				return usageError("baha whoami does not accept arguments", "Run 'baha whoami'.")
			}
			cfg, err := operatorauth.ConfigFromEnv()
			if err != nil {
				return err
			}
			principal, err := operatorauth.VerifySession(ctx, cfg)
			if err != nil {
				if errors.Is(err, operatorauth.ErrAuthenticationRequired) {
					return usageError("no valid operator session", "Run 'baha login'.")
				}
				return err
			}
			fmt.Fprintf(out, "Issuer: %s\nSubject: %s\n", principal.Issuer, principal.Subject)
			if principal.Assurance != "" {
				fmt.Fprintf(out, "Assurance: %s\n", principal.Assurance)
			}
			if len(principal.Methods) > 0 {
				fmt.Fprintf(out, "Authentication methods: %v\n", principal.Methods)
			}
			return nil
		},
	}
}
