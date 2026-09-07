package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mcpdev80/baseharbor/internal/config"
	"github.com/mcpdev80/baseharbor/internal/health"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func runtimeUp(parent context.Context, out io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
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
	fmt.Fprintln(out, "BaseHarbor control-plane runtime started")
	fmt.Fprintln(out, "next: run 'baha status' and 'baha doctor'")
	return nil
}

func runtimeDown(parent context.Context, out io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, time.Minute)
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
	fmt.Fprintln(out, "BaseHarbor control-plane runtime stopped")
	return nil
}

func runtimeStatus(parent context.Context, out io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
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
	fmt.Fprint(out, status)

	checks := health.RuntimeChecks()
	if len(checks) == 0 {
		return nil
	}
	formatted, ok := health.Format(checks)
	fmt.Fprint(out, formatted)
	if !ok {
		return errors.New("runtime is running but not ready")
	}
	return nil
}

func initConfig(out io.Writer) error {
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
	fmt.Fprintf(out, "created %s\n", path)
	fmt.Fprintln(out, "next: run 'baha doctor'")
	return nil
}
