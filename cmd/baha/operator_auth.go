package main

import (
	"context"
	"errors"
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
			filtered, environment, err := extractApplicationEnvironment(args, "whoami")
			if err != nil {
				return err
			}
			if len(filtered) != 0 || strings.TrimSpace(environment) == "" {
				return usageError("baha whoami requires only -e/--environment", "Example: baha whoami -e test")
			}
			if !operatorauth.ManagedEnvironment(environment) {
				fmt.Fprintln(out, "Mode: trusted-local")
				fmt.Fprintln(out, "Identity: local-operator")
				return nil
			}
			target, err := effectiveTarget(ctx)
			if err != nil {
				return err
			}
			stored, found, err := resolveStoredOperatorAuthBoundary(target.Name, environment)
			if err != nil {
				return err
			}
			if !found {
				return usageError("operator authentication is not configured", "Run a command against this environment interactively to bootstrap OIDC, or configure it first.")
			}
			principal, err := operatorauth.VerifySession(ctx, target.Name, environment, stored)
			if err != nil {
				if errors.Is(err, operatorauth.ErrAuthenticationRequired) {
					return usageError("no valid operator session", "Run 'baha login -e "+environment+"'.")
				}
				return err
			}
			fmt.Fprintf(out, "Target: %s\nEnvironment: %s\nIssuer: %s\nSubject: %s\n", target.Name, environment, principal.Issuer, principal.Subject)
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
