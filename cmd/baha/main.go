package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/mcpdev80/baseharbor/internal/config"
	"github.com/mcpdev80/baseharbor/internal/health"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printHelp()
		return nil
	}

	switch args[0] {
	case "version", "--version", "-v":
		fmt.Printf("baha %s (commit %s, built %s)\n", version, commit, date)
		return nil
	case "init":
		return initConfig()
	case "doctor":
		formatted, ok := health.Format(health.Doctor())
		fmt.Print(formatted)
		if !ok {
			return errors.New("one or more checks failed")
		}
		return nil
	case "help", "--help", "-h":
		printHelp()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func initConfig() error {
	const path = config.DefaultFile
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	cfg := config.Default()
	if err := os.WriteFile(path, []byte(cfg.YAML()), 0o600); err != nil {
		return err
	}

	fmt.Printf("created %s\n", path)
	fmt.Println("next: run 'baha doctor'")
	return nil
}

func printHelp() {
	fmt.Print(`BaseHarbor CLI

Usage:
  baha <command>

Commands:
  init       Create a minimal BaseHarbor configuration
  doctor     Check whether the host is ready for BaseHarbor
  version    Print build version
  help       Show this help
`)
}
