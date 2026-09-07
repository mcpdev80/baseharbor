package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/mcpdev80/baseharbor/internal/config"
	"github.com/mcpdev80/baseharbor/internal/health"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
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
	case "up":
		return runtimeUp()
	case "down":
		return runtimeDown()
	case "status":
		return runtimeStatus()
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

func runtimeUp() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		return err
	}
	files, err := bhruntime.EnsureFiles("")
	if err != nil {
		return err
	}
	if err := compose.Config(ctx, files.Compose, files.Env); err != nil {
		return err
	}
	if err := compose.Up(ctx, files.Compose, files.Env); err != nil {
		return err
	}
	fmt.Println("BaseHarbor runtime started")
	fmt.Println("next: run 'baha status' and 'baha doctor'")
	return nil
}

func runtimeDown() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		return err
	}
	files, err := bhruntime.ExistingFiles("")
	if err != nil {
		return fmt.Errorf("runtime is not initialized: %w", err)
	}
	if err := compose.Down(ctx, files.Compose, files.Env); err != nil {
		return err
	}
	fmt.Println("BaseHarbor runtime stopped")
	return nil
}

func runtimeStatus() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		return err
	}
	files, err := bhruntime.ExistingFiles("")
	if err != nil {
		return fmt.Errorf("runtime is not initialized: %w", err)
	}
	status, err := compose.Status(ctx, files.Compose, files.Env)
	if err != nil {
		return err
	}
	fmt.Print(status)
	return nil
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
  up         Start the local BaseHarbor runtime
  down       Stop the local BaseHarbor runtime
  status     Show runtime container status
  doctor     Check whether the host and runtime are ready
  version    Print build version
  help       Show this help
`)
}
