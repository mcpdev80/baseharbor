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
		Usage:   "baha login -e ENV|--environment ENV [--init-tenant] [--tenant-slug SLUG] [--tenant-name NAME] [--ca FILE] [--auth-code-file FILE] [--no-browser]",
		Long:    "Authenticates through Authorization Code + PKCE. --init-tenant is an explicit first-owner operation, restricted to the locally owned management Core after real OIDC login; it creates exactly one initial tenant and editor membership. --ca trusts the managed CA the Core bootstrapped without OS system-trust surgery (BASEHARBOR_OPERATOR_OIDC_CA_FILE is the state-wide alias). On non-browser hosts complete the PKCE flow headless with --auth-code-file (or BASEHARBOR_OPERATOR_OIDC_AUTH_CODE_FILE) and --no-browser; a fixed loopback callback port is honored via BASEHARBOR_OPERATOR_OIDC_CALLBACK_PORT. Existing trusted local dev without an operator OIDC configuration remains login-free.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			loginArgs, initTenant, tenantSlug, tenantName, err := parseInitialTenantLoginArgs(args)
			if err != nil {
				return err
			}
			caFiltered, caFile, authCodeFile, noBrowser, err := parseLoginTrustArgs(loginArgs)
			if err != nil {
				return err
			}
			filtered, environment, err := extractApplicationEnvironment(caFiltered, "login")
			if err != nil {
				return err
			}
			if len(filtered) != 0 {
				return usageError("baha login accepts only -e/--environment, --init-tenant, --ca, --auth-code-file and --no-browser", "Example: baha login -e test --init-tenant")
			}
			if strings.TrimSpace(environment) == "" {
				return usageError("baha login requires -e/--environment", "Example: baha login -e test")
			}
			target, err := effectiveTarget(ctx)
			if err != nil {
				return err
			}
			if !operatorauth.ManagedEnvironment(environment) && !initTenant {
				_, found, err := resolveStoredOperatorAuthBoundary(target.Name, environment)
				if err != nil {
					return err
				}
				if !found {
					fmt.Fprintln(out, "Development uses trusted local operator mode; no login is required.")
					return nil
				}
			}
			if initTenant {
				if _, err := initialOperatorLocalCoreFiles(ctx, target); err != nil {
					return err
				}
			}
			cfg, err := resolveOperatorAuthBoundaryConfig(ctx, target.Name, environment)
			if err != nil {
				return err
			}
			principal, err := operatorauth.Login(ctx, target.Name, environment, cfg, out, operatorauth.TrustOptions{
				ExplicitCaFile: caFile,
				AuthCodeFile:   authCodeFile,
				NoBrowser:      noBrowser,
			})
			if err != nil {
				return err
			}
			if initTenant {
				result, err := initializeLocalOperatorTenant(ctx, target, principal, tenantSlug, tenantName)
				if err != nil {
					_ = operatorauth.ClearSession(target.Name, environment)
					return fmt.Errorf("initialize local operator tenant: %w", err)
				}
				if cfg.Provider == "managed-keycloak" {
					if err := finalizeManagedOperatorKeycloak(ctx, target.Name, environment); err != nil {
						return fmt.Errorf("finalize managed operator registration: %w", err)
					}
				}
				fmt.Fprintf(out, "Initial tenant: %s (created=%t)\n", result.TenantID, result.Created)
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

func parseOperatorLoginArgs(args []string) ([]string, bool, error) {
	var initialize bool
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg != "--init-tenant" {
			filtered = append(filtered, arg)
			continue
		}
		if initialize {
			return nil, false, usageError("--init-tenant may be specified only once", "Use baha login -e ENV --init-tenant.")
		}
		initialize = true
	}
	return filtered, initialize, nil
}

func parseInitialTenantLoginArgs(args []string) ([]string, bool, string, string, error) {
	slug, name := "baseharbor", "BaseHarbor"
	filtered := make([]string, 0, len(args))
	seenSlug, seenName := false, false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg != "--tenant-name" && arg != "--tenant-slug" {
			filtered = append(filtered, arg)
			continue
		}
		if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
			return nil, false, "", "", usageError(arg+" requires a value", "Provide a valid tenant name.")
		}
		i++
		switch arg {
		case "--tenant-name":
			if seenName {
				return nil, false, "", "", usageError("duplicate --tenant-name", "Specify the tenant name once.")
			}
			name, seenName = args[i], true
		case "--tenant-slug":
			if seenSlug {
				return nil, false, "", "", usageError("duplicate --tenant-slug", "Specify the tenant slug once.")
			}
			slug, seenSlug = args[i], true
		}
	}
	remaining, initialize, err := parseOperatorLoginArgs(filtered)
	if err != nil {
		return nil, false, "", "", err
	}
	if (seenSlug || seenName) && !initialize {
		return nil, false, "", "", usageError("tenant options require --init-tenant", "Add --init-tenant when customizing the first tenant.")
	}
	return remaining, initialize, slug, name, nil
}

// parseLoginTrustArgs extracts the headless-completion and trust flags that
// baha login supports on non-browser hosts: --ca/--auth-code-file (each with a
// required value, or =VALUE) and the --no-browser companion. It returns the
// environment-cleaned args plus the optional values so the caller can strip
// them before generic argument parsing. --no-browser is captured into noBrowser
// and consumed here so it never reaches the generic leftover check.
func parseLoginTrustArgs(args []string) (remaining []string, caFile, authCodeFile string, noBrowser bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--no-browser":
			noBrowser = true
		case arg == "--ca":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return nil, "", "", false, usageError("--ca requires a CA certificate file", "Provide a PEM file, e.g. --ca ~/.local/share/baseharbor/trust/managed-operator-ca.pem.")
			}
			i++
			caFile = strings.TrimSpace(args[i])
		case arg == "--auth-code-file":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return nil, "", "", false, usageError("--auth-code-file requires a file containing the authorization code", "Complete the headless login by writing the returned code to this file.")
			}
			i++
			authCodeFile = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--ca="):
			caFile = strings.TrimSpace(strings.TrimPrefix(arg, "--ca="))
		case strings.HasPrefix(arg, "--auth-code-file="):
			authCodeFile = strings.TrimSpace(strings.TrimPrefix(arg, "--auth-code-file="))
		default:
			remaining = append(remaining, arg)
		}
	}
	return remaining, caFile, authCodeFile, noBrowser, nil
}
