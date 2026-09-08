package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/openbao"
)

func appSecretCommand(store application.Store) *cli.Command {
	service := applicationsecret.New(store)
	command := &cli.Command{
		Name:    "secret",
		Summary: "Manage application secret values without printing them",
		Usage:   "baha app secret <command> [options]",
		Long:    "Stores application secret values in the application's isolated managed secret namespace. Secret values are accepted only through stdin and are never rendered by this command group.",
	}
	command.Children = []*cli.Command{
		{
			Name:    "set",
			Summary: "Create or replace one secret value from stdin",
			Usage:   "baha app secret set NAME KEY --stdin",
			Long:    "Reads one UTF-8 secret value from stdin and stores it through the application secret service. The value is never accepted as a command-line argument and is not printed. Input is preserved exactly, including trailing newlines.\n\nExample:\n  printf '%s' 'secret-value' | baha app secret set demo API_TOKEN --stdin",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				name, key, err := parseSecretSetArgs(args)
				if err != nil {
					return err
				}
				value, err := readSecretValue(os.Stdin)
				if err != nil {
					return err
				}
				if err := service.Set(ctx, name, key, value); err != nil {
					return err
				}
				m, _, err := store.Load(name)
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "Secret %s updated for application %s (%s).\n", key, m.Name, m.Environment)
				return nil
			},
		},
		{
			Name:    "list",
			Summary: "List secret key names without values",
			Usage:   "baha app secret list NAME",
			Long:    "Lists only managed secret key names. Secret values are never returned.",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				if len(args) != 1 {
					return usageError("baha app secret list requires exactly one NAME", "Example: baha app secret list demo")
				}
				items, err := service.List(ctx, args[0])
				if err != nil {
					return err
				}
				configured := make([]applicationsecret.Metadata, 0, len(items))
				for _, item := range items {
					if item.Present {
						configured = append(configured, item)
					}
				}
				if len(configured) == 0 {
					fmt.Fprintln(out, "No application secrets configured.")
					return nil
				}
				fmt.Fprintln(out, "KEY")
				for _, item := range configured {
					fmt.Fprintln(out, item.Name)
				}
				return nil
			},
		},
		{
			Name:    "delete",
			Summary: "Permanently delete one secret and all of its KV versions",
			Usage:   "baha app secret delete NAME KEY [--yes]",
			Long:    "Without --yes, validates the application secret scope and prints a read-only deletion preview. With --yes, permanently removes the selected secret document and all of its managed versions/metadata.",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				name, key, yes, err := parseSecretDeleteArgs(args)
				if err != nil {
					return err
				}
				items, err := service.List(ctx, name)
				if err != nil {
					return err
				}
				found := false
				for _, item := range items {
					if item.Name == key && item.Present {
						found = true
						break
					}
				}
				if !found {
					return openbao.ErrApplicationSecretNotFound
				}
				m, _, err := store.Load(name)
				if err != nil {
					return err
				}
				if !yes {
					fmt.Fprintf(out, "Would permanently delete secret %s from application %s (%s), including all managed versions.\n", key, m.Name, m.Environment)
					fmt.Fprintln(out, "No changes were made. Re-run with --yes to confirm.")
					return nil
				}
				if err := service.Delete(ctx, name, key); err != nil {
					return err
				}
				fmt.Fprintf(out, "Secret %s permanently deleted from application %s (%s).\n", key, m.Name, m.Environment)
				return nil
			},
		},
	}
	return command
}

func parseSecretSetArgs(args []string) (string, string, error) {
	var positional []string
	stdin := false
	for _, arg := range args {
		switch {
		case arg == "--stdin":
			stdin = true
		case strings.HasPrefix(arg, "-"):
			return "", "", usageError("unknown option "+arg, "Usage: baha app secret set NAME KEY --stdin")
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) != 2 {
		return "", "", usageError("baha app secret set requires NAME and KEY", "Example: printf '%s' 'value' | baha app secret set demo API_TOKEN --stdin")
	}
	if !stdin {
		return "", "", usageError("baha app secret set requires --stdin", "Secret values are never accepted as command-line arguments.")
	}
	return positional[0], positional[1], nil
}

func parseSecretDeleteArgs(args []string) (string, string, bool, error) {
	var positional []string
	yes := false
	for _, arg := range args {
		switch {
		case arg == "--yes":
			yes = true
		case strings.HasPrefix(arg, "-"):
			return "", "", false, usageError("unknown option "+arg, "Usage: baha app secret delete NAME KEY [--yes]")
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) != 2 {
		return "", "", false, usageError("baha app secret delete requires NAME and KEY", "Example: baha app secret delete demo API_TOKEN --yes")
	}
	return positional[0], positional[1], yes, nil
}

func readSecretValue(reader io.Reader) ([]byte, error) {
	value, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
	if err != nil {
		return nil, errors.New("read application secret from stdin failed")
	}
	if len(value) > 1<<20 {
		return nil, errors.New("application secret value exceeds the 1048576-byte limit")
	}
	if len(value) == 0 {
		return nil, errors.New("application secret value from stdin is empty")
	}
	return value, nil
}
