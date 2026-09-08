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
		Long:    "Stores application secret values in the application's isolated managed secret namespace. Inside a repository containing baseharbor.yaml the application NAME may be omitted.",
	}
	command.Children = []*cli.Command{
		{
			Name:    "set",
			Summary: "Create or replace one secret value from stdin or a file",
			Usage:   "baha app secret set [NAME] KEY (--stdin | --file PATH)",
			Long:    "Reads one secret value from stdin or directly from a file. In a repository use 'baha app secret set KEY --stdin' or 'baha app secret set TLS_KEY_FILE --file ./key.pem'. Secret values are never accepted as command-line arguments or printed.",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				name, key, err := parseSecretSetArgs(args)
				if err != nil {
					return err
				}
				resolved, err := resolveSecretApplication(store, name, "secret set")
				if err != nil {
					return err
				}
				value, err := readSecretSetValue(args, os.Stdin)
				if err != nil {
					return err
				}
				if err := service.Set(ctx, resolved.Manifest.Name, key, value); err != nil {
					return err
				}
				fmt.Fprintf(out, "Secret %s updated for application %s (%s).\n", key, resolved.Manifest.Name, resolved.Manifest.Environment)
				return nil
			},
		},
		{
			Name:    "list",
			Summary: "List secret key names without values",
			Usage:   "baha app secret list [NAME]",
			Long:    "Lists only managed secret key names. Secret values are never returned.",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				if len(args) > 1 {
					return usageError("baha app secret list accepts at most one NAME", "Inside an application repository omit NAME.")
				}
				name := ""
				if len(args) == 1 {
					name = args[0]
				}
				resolved, err := resolveSecretApplication(store, name, "secret list")
				if err != nil {
					return err
				}
				items, err := service.List(ctx, resolved.Manifest.Name)
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
			Usage:   "baha app secret delete [NAME] KEY [--yes]",
			Long:    "Without --yes, validates the application secret scope and prints a read-only deletion preview. Inside a repository omit NAME.",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				name, key, yes, err := parseSecretDeleteArgs(args)
				if err != nil {
					return err
				}
				resolved, err := resolveSecretApplication(store, name, "secret delete")
				if err != nil {
					return err
				}
				appName := resolved.Manifest.Name
				items, err := service.List(ctx, appName)
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
				if !yes {
					fmt.Fprintf(out, "Would permanently delete secret %s from application %s (%s), including all managed versions.\n", key, appName, resolved.Manifest.Environment)
					fmt.Fprintln(out, "No changes were made. Re-run with --yes to confirm.")
					return nil
				}
				if err := service.Delete(ctx, appName, key); err != nil {
					return err
				}
				fmt.Fprintf(out, "Secret %s permanently deleted from application %s (%s).\n", key, appName, resolved.Manifest.Environment)
				return nil
			},
		},
	}
	command.Children = append(command.Children, appSecretTLSSetCommand(store, service))
	return command
}

func resolveSecretApplication(store application.Store, name, command string) (resolvedApplication, error) {
	if name == "" {
		return resolveApplication(store, nil, command)
	}
	return resolveApplication(store, []string{name}, command)
}

func parseSecretSetArgs(args []string) (string, string, error) {
	var positional []string
	stdin := false
	filePath := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--stdin":
			stdin = true
		case arg == "--file":
			if i+1 >= len(args) {
				return "", "", usageError("--file requires a path", "Usage: baha app secret set [NAME] KEY (--stdin | --file PATH)")
			}
			i++
			filePath = args[i]
		case strings.HasPrefix(arg, "--file="):
			filePath = strings.TrimPrefix(arg, "--file=")
		case strings.HasPrefix(arg, "-"):
			return "", "", usageError("unknown option "+arg, "Usage: baha app secret set [NAME] KEY (--stdin | --file PATH)")
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) < 1 || len(positional) > 2 {
		return "", "", usageError("baha app secret set requires KEY and accepts optional NAME", "Inside a repository: baha app secret set API_TOKEN --stdin")
	}
	if stdin == (filePath != "") {
		return "", "", usageError("baha app secret set requires exactly one input source", "Use either --stdin or --file PATH.")
	}
	if filePath != "" && strings.TrimSpace(filePath) == "" {
		return "", "", usageError("--file path is empty", "Provide a readable file path.")
	}
	if len(positional) == 1 {
		return "", positional[0], nil
	}
	return positional[0], positional[1], nil
}

func secretSetFilePath(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "--file" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(args[i], "--file=") {
			return strings.TrimPrefix(args[i], "--file=")
		}
	}
	return ""
}

func readSecretSetValue(args []string, stdin io.Reader) ([]byte, error) {
	if path := secretSetFilePath(args); path != "" {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open application secret file: %w", err)
		}
		defer file.Close()
		return readSecretValue(file)
	}
	return readSecretValue(stdin)
}

func parseSecretDeleteArgs(args []string) (string, string, bool, error) {
	var positional []string
	yes := false
	for _, arg := range args {
		switch {
		case arg == "--yes":
			yes = true
		case strings.HasPrefix(arg, "-"):
			return "", "", false, usageError("unknown option "+arg, "Usage: baha app secret delete [NAME] KEY [--yes]")
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) < 1 || len(positional) > 2 {
		return "", "", false, usageError("baha app secret delete requires KEY and accepts optional NAME", "Inside a repository: baha app secret delete API_TOKEN --yes")
	}
	if len(positional) == 1 {
		return "", positional[0], yes, nil
	}
	return positional[0], positional[1], yes, nil
}

func readSecretValue(reader io.Reader) ([]byte, error) {
	value, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
	if err != nil {
		return nil, errors.New("read application secret failed")
	}
	if len(value) > 1<<20 {
		return nil, errors.New("application secret value exceeds the 1048576-byte limit")
	}
	if len(value) == 0 {
		return nil, errors.New("application secret value is empty")
	}
	return value, nil
}
