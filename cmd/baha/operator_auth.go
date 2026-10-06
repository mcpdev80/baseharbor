package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

func operatorLoginCommand() *cli.Command {
	return &cli.Command{
		Name:    "login",
		Summary: "Authenticate the BaseHarbor operator through OIDC",
		Usage:   "baha login -e ENV|--environment ENV",
		Long:    "Authenticates against the operator OIDC configuration stored for the effective Target/Environment boundary. The CLI uses Authorization Code + PKCE and persists only a short-lived owner-only local session. Trusted local dev does not require login.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, environment, err := extractApplicationEnvironment(args, "login")
			if err != nil {
				return err
			}
			if len(filtered) != 0 {
				return usageError("baha login accepts only -e/--environment", "Example: baha login -e test")
			}
			if strings.TrimSpace(environment) == "" {
				return usageError("baha login requires -e/--environment", "Example: baha login -e test")
			}
			if !operatorauth.ManagedEnvironment(environment) {
				fmt.Fprintln(out, "Development uses trusted local operator mode; no login is required.")
				return nil
			}
			target, err := effectiveTarget(ctx)
			if err != nil {
				return err
			}
			cfg, err := resolveOperatorAuthBoundaryConfig(ctx, target.Name, environment)
			if err != nil {
				return err
			}
			principal, err := operatorauth.Login(ctx, target.Name, environment, cfg, out)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Authenticated operator: %s\nIssuer: %s\nTarget: %s\nEnvironment: %s\n", principal.Subject, principal.Issuer, target.Name, environment)
			return nil
		},
	}
}

func operatorLogoutCommand() *cli.Command {
	return &cli.Command{
		Name:    "logout",
		Summary: "Remove a local BaseHarbor operator session",
		Usage:   "baha logout -e ENV|--environment ENV",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, environment, err := extractApplicationEnvironment(args, "logout")
			if err != nil {
				return err
			}
			if len(filtered) != 0 || strings.TrimSpace(environment) == "" {
				return usageError("baha logout requires only -e/--environment", "Example: baha logout -e prod")
			}
			target, err := effectiveTarget(ctx)
			if err != nil {
				return err
			}
			if err := operatorauth.ClearSession(target.Name, environment); err != nil {
				return err
			}
			fmt.Fprintf(out, "Operator session removed for %s/%s.\n", target.Name, environment)
			return nil
		},
	}
}

func operatorWhoAmICommand() *cli.Command {
	return &cli.Command{
		Name:    "whoami",
		Summary: "Show the authenticated BaseHarbor operator identity",
		Usage:   "baha whoami -e ENV|--environment ENV",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			outputArgs, format, err := parseReadOutputArgs(args, "whoami")
			if err != nil {
				return err
			}
			args = outputArgs
			filtered, environment, err := extractApplicationEnvironment(args, "whoami")
			if err != nil {
				return err
			}
			if len(filtered) != 0 || strings.TrimSpace(environment) == "" {
				return usageError("baha whoami requires only -e/--environment", "Example: baha whoami -e test")
			}
			result, err := inspectOperatorIdentity(ctx, environment)
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			actor := result.Actor
			if actor.Mode == "trusted-local" {
				fmt.Fprintln(out, "Mode: trusted-local\nIdentity: local-operator")
				return nil
			}
			fmt.Fprintf(out, "Target: %s\nEnvironment: %s\nIssuer: %s\nSubject: %s\n", result.Target, result.Environment, actor.Issuer, actor.Subject)
			if actor.Assurance != "" {
				fmt.Fprintf(out, "Assurance: %s\n", actor.Assurance)
			}
			if len(actor.Methods) > 0 {
				fmt.Fprintf(out, "Authentication methods: %v\n", actor.Methods)
			}
			return nil
		},
	}
}
