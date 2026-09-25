package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/health"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"io"
	"strings"
	"time"
)

func runtimeUp(parent context.Context, out io.Writer) error {
	return runtimeUpExisting(parent, out, "")
}

func runtimeUpWithPorts(parent context.Context, out io.Writer, ports bhruntime.Ports) error {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()

	compose, files, err := startControlPlaneRuntime(ctx, out, ports)
	if err != nil {
		return err
	}
	if err := waitForOpenBaoExecReady(ctx, compose, files); err != nil {
		return fmt.Errorf("wait for OpenBao control-plane readiness: %w", err)
	}
	if state, inspectErr := platformopenbao.Inspect(ctx, compose, files); inspectErr == nil && state.Initialized && !state.Sealed {
		if managerErr := platformopenbao.CheckManager(ctx, compose, files); managerErr == nil {
			if err := reconcileControlPlaneServiceAccess(ctx, compose, files); err != nil {
				return fmt.Errorf("reconcile control-plane service access: %w", err)
			}
		}
	}
	if err := resumeSharedPlatformRuntime(ctx, compose, out); err != nil {
		return fmt.Errorf("resume shared platform runtime: %w", err)
	}
	fmt.Fprintln(out, "BaseHarbor control-plane runtime started")
	fmt.Fprintln(out, "next: run 'baha status' and 'baha doctor'")
	return nil
}

func waitForOpenBaoExecReady(ctx context.Context, compose bhruntime.Compose, files bhruntime.Files) error {
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for {
		if _, err := platformopenbao.Inspect(ctx, compose, files); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return lastErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func runtimeUpExisting(parent context.Context, out io.Writer, recoveryFile string) error {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()

	files, err := existingTargetRuntimeFiles(parent)
	if err != nil {
		return fmt.Errorf("runtime is not initialized: %w", err)
	}
	if strings.TrimSpace(recoveryFile) == "" {
		checks := health.RuntimeChecksForFiles(files)
		if len(checks) > 0 {
			_, ready := health.Format(checks)
			if ready {
				fmt.Fprintln(out, "BaseHarbor control-plane runtime already READY. No changes.")
				return nil
			}
		}
	}

	compose, err := startExistingControlPlaneRuntime(ctx, files)
	if err != nil {
		return err
	}
	if err := verifyExistingControlPlaneAfterStart(ctx, compose, files, strings.TrimSpace(recoveryFile), out); err != nil {
		return err
	}
	if err := reconcileControlPlaneServiceAccess(ctx, compose, files); err != nil {
		return fmt.Errorf("reconcile control-plane service access: %w", err)
	}
	if err := resumeSharedPlatformRuntime(ctx, compose, out); err != nil {
		return fmt.Errorf("resume shared platform runtime after verified control plane: %w", err)
	}
	fmt.Fprintln(out, "BaseHarbor control-plane runtime started and ready")
	fmt.Fprintln(out, "next: run 'baha status' and 'baha doctor'")
	return nil
}

func reconcileControlPlaneServiceAccess(ctx context.Context, compose bhruntime.Compose, files bhruntime.Files) error {
	state, err := platformopenbao.Inspect(ctx, compose, files)
	if err != nil {
		return err
	}
	if !state.Initialized || state.Sealed {
		return nil
	}
	if err := platformopenbao.CheckManager(ctx, compose, files); err != nil {
		return err
	}
	issuer := platformopenbao.NewServiceIssuer(compose, files)
	status, err := issuer.Status(ctx)
	if err != nil {
		return err
	}
	if !status.Ready {
		return errors.New("managed service issuer is not ready")
	}
	if err := bhruntime.EnsureServiceAccess(ctx, issuer, files); err != nil {
		return err
	}
	if err := compose.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return err
	}
	return compose.UpProject(ctx, files.Project, files.Compose, files.Env)
}

func startControlPlaneRuntime(ctx context.Context, out io.Writer, ports bhruntime.Ports) (bhruntime.Compose, bhruntime.Files, error) {
	target, files, err := ensureTargetRuntimeFiles(ctx, ports)
	if err != nil {
		return bhruntime.Compose{}, bhruntime.Files{}, err
	}
	compose, err := detectComposeForTarget(ctx, target)
	if err != nil {
		return bhruntime.Compose{}, bhruntime.Files{}, err
	}
	if err := compose.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return bhruntime.Compose{}, bhruntime.Files{}, err
	}
	if err := compose.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return bhruntime.Compose{}, bhruntime.Files{}, err
	}
	return compose, files, nil
}

func startExistingControlPlaneRuntime(ctx context.Context, files bhruntime.Files) (bhruntime.Compose, error) {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return bhruntime.Compose{}, err
	}
	compose, err := detectComposeForTarget(ctx, target)
	if err != nil {
		return bhruntime.Compose{}, err
	}
	if err := compose.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return bhruntime.Compose{}, err
	}
	if err := compose.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return bhruntime.Compose{}, err
	}
	return compose, nil
}

func verifyExistingControlPlaneAfterStart(ctx context.Context, compose bhruntime.Compose, files bhruntime.Files, recoveryFile string, out io.Writer) error {
	var state platformopenbao.State
	var inspectErr error
	deadline := time.Now().Add(30 * time.Second)
	for {
		state, inspectErr = platformopenbao.Inspect(ctx, compose, files)
		if inspectErr == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("verify OpenBao after control-plane start: %w", inspectErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}

	if !state.Initialized {
		fmt.Fprintln(out, "BaseHarbor control-plane runtime started; OpenBao is not initialized yet.")
		fmt.Fprintln(out, "next: run 'baha openbao bootstrap --recovery-file PATH'")
		return nil
	}
	if state.Sealed {
		if recoveryFile == "" {
			return usageError(
				"OpenBao is initialized but sealed after the control-plane restart",
				"Re-run 'baha up --recovery-file /secure/openbao-recovery.json'. BaseHarbor will not unseal OpenBao without operator-held recovery material.",
			)
		}
		fmt.Fprintln(out, "OpenBao is sealed; unsealing from the operator recovery file...")
		if err := platformopenbao.Unseal(ctx, compose, files, recoveryFile); err != nil {
			return fmt.Errorf("unseal OpenBao after control-plane start: %w", err)
		}
	}
	if err := platformopenbao.CheckManager(ctx, compose, files); err != nil {
		return fmt.Errorf("verify OpenBao manager authentication after control-plane start: %w", err)
	}
	if err := reconcileControlPlaneServiceAccess(ctx, compose, files); err != nil {
		return fmt.Errorf("reconcile control-plane service access after start: %w", err)
	}

	var formatted string
	var ok bool
	readinessDeadline := time.Now().Add(30 * time.Second)
	for {
		formatted, ok = health.Format(health.RuntimeChecksForFiles(files))
		if ok {
			fmt.Fprint(out, formatted)
			return nil
		}
		if time.Now().After(readinessDeadline) {
			fmt.Fprint(out, formatted)
			detail := strings.TrimSpace(formatted)
			if detail == "" {
				return errors.New("control-plane runtime started but did not become ready")
			}
			return fmt.Errorf("control-plane runtime started but did not become ready: %s", detail)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}
