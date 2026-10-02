package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/health"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const (
	controlPlaneStartTimeout        = 10 * time.Minute
	controlPlaneComposeStartTimeout = 8 * time.Minute
)

func runtimeUp(parent context.Context, out io.Writer) error {
	return runtimeUpExisting(parent, out, "")
}

func runtimeUpWithPorts(parent context.Context, out io.Writer, ports bhruntime.Ports) error {
	ctx, cancel := context.WithTimeout(parent, controlPlaneStartTimeout)
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
			if err := reconcileControlPlaneServiceAccess(ctx, compose, files, ""); err != nil {
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

func waitForOpenBaoExecReady(ctx context.Context, compose bhruntime.RuntimeProvider, files bhruntime.Files) error {
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
	ctx, cancel := context.WithTimeout(parent, controlPlaneStartTimeout)
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

	resolvedRecoveryFile, _, err := resolveTargetRecoveryFile(ctx, recoveryFile)
	if err != nil {
		return err
	}
	compose, err := startExistingControlPlaneRuntime(ctx, files)
	if err != nil {
		return err
	}
	if err := verifyExistingControlPlaneAfterStart(ctx, compose, files, resolvedRecoveryFile, out); err != nil {
		return err
	}
	if _, found, err := bhruntime.LoadControlPlaneCredentialRotation(files); err != nil {
		return fmt.Errorf("inspect pending control-plane credential rotation: %w", err)
	} else if found {
		fmt.Fprintln(out, "Resuming pending control-plane credential rotation...")
		if err := rotateControlPlaneDatabaseCredentials(ctx, compose, files, resolvedRecoveryFile); err != nil {
			return fmt.Errorf("resume control-plane credential rotation: %w", err)
		}
	}
	if err := reconcileControlPlaneServiceAccess(ctx, compose, files, resolvedRecoveryFile); err != nil {
		return fmt.Errorf("reconcile control-plane service access: %w", err)
	}
	if err := resumeSharedPlatformRuntime(ctx, compose, out); err != nil {
		return fmt.Errorf("resume shared platform runtime after verified control plane: %w", err)
	}
	fmt.Fprintln(out, "BaseHarbor control-plane runtime started and ready")
	fmt.Fprintln(out, "next: run 'baha status' and 'baha doctor'")
	return nil
}

func reconcileControlPlaneServiceAccess(ctx context.Context, compose bhruntime.RuntimeProvider, files bhruntime.Files, recoveryFile string) error {
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
	bootstrapRestart, err := bhruntime.ControlPlaneServiceAccessNeedsBootstrapRestart(files)
	if err != nil {
		return err
	}
	if err := bhruntime.EnsureServiceAccess(ctx, issuer, files); err != nil {
		return err
	}
	if err := compose.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return err
	}
	if bootstrapRestart {
		if err := compose.DownProject(ctx, files.Project, files.Compose, files.Env); err != nil {
			return fmt.Errorf("restart control plane for managed PKI transition: %w", err)
		}
	}
	if err := compose.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return err
	}
	if !bootstrapRestart {
		for _, member := range []string{"postgres-member-1", "postgres-member-2", "postgres-member-3"} {
			script := `python3 - <<'PY'
import urllib.request
req = urllib.request.Request("http://127.0.0.1:8008/reload", data=b"", method="POST")
with urllib.request.urlopen(req, timeout=5) as response:
    if response.status < 200 or response.status >= 300:
        raise SystemExit("unexpected Patroni reload status %s" % response.status)
PY`
			if _, err := compose.ExecProject(ctx, files.Project, files.Compose, files.Env, member, "sh", "-ec", script); err != nil {
				return fmt.Errorf("reload PostgreSQL native TLS material on %s: %w", member, err)
			}
		}
	}

	// The initial bootstrap trust transition restarts OpenBao so its PostgreSQL
	// client loads the managed PostgreSQL CA. Later OpenBao listener certificate
	// rotations are handled by OpenBao 2.7 tls_auto_reload.
	if err := waitForOpenBaoExecReady(ctx, compose, files); err != nil {
		return fmt.Errorf("wait for OpenBao after native TLS reconcile: %w", err)
	}
	state, err = platformopenbao.Inspect(ctx, compose, files)
	if err != nil {
		return err
	}
	if state.Sealed {
		if strings.TrimSpace(recoveryFile) == "" {
			return usageError(
				"OpenBao restarted while enabling native TLS and is sealed",
				"Re-run with '--recovery-file PATH' using the recovery material created during bootstrap.",
			)
		}
		if err := platformopenbao.Unseal(ctx, compose, files, recoveryFile); err != nil {
			return fmt.Errorf("unseal OpenBao after native TLS reconcile: %w", err)
		}
	}
	if err := platformopenbao.CheckManager(ctx, compose, files); err != nil {
		return fmt.Errorf("verify OpenBao manager after native TLS reconcile: %w", err)
	}

	policy, err := serviceaccess.Resolve("prod", "openbao", serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	policy.ServerName = "openbao"
	material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(filepath.Dir(files.Compose), "providers", "openbao", "service-access", "pki"))
	if err != nil {
		return fmt.Errorf("load OpenBao native TLS material: %w", err)
	}
	client, err := serviceaccess.NewHTTPClient(material, false)
	if err != nil {
		return err
	}
	cfg, err := bhruntime.LoadConfig(files.Env)
	if err != nil {
		return err
	}
	endpoint, err := serviceaccess.LoopbackHTTPSURL(cfg.OpenBaoPort)
	if err != nil {
		return err
	}
	verifyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := serviceaccess.WaitHTTPS(verifyCtx, client, endpoint, "/v1/sys/health"); err != nil {
		return fmt.Errorf("verify OpenBao native HTTPS endpoint: %w", err)
	}
	if _, err := compose.ExecProject(ctx, files.Project, files.Compose, files.Env, "postgres-admin",
		"sh", "-ec",
		"pg_isready -h postgres -p 5432 -U \"$BASEHARBOR_POSTGRES_USER\" -d \"$BASEHARBOR_POSTGRES_DB\""); err != nil {
		return fmt.Errorf("verify control-plane PostgreSQL after native TLS reconcile: %w", err)
	}

	// Retire previous trust only after both stable service paths have accepted
	// the replacement leaves. Re-project the new-only CA bundles afterwards
	// and verify the operator path one more time.
	if err := bhruntime.RetireControlPlaneServiceAccessOverlap(ctx, issuer, files); err != nil {
		return err
	}
	if err := platformopenbao.CheckManager(ctx, compose, files); err != nil {
		return fmt.Errorf("verify OpenBao manager after CA retirement: %w", err)
	}
	postRetirePolicy, err := serviceaccess.Resolve("prod", "openbao", serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	postRetirePolicy.ServerName = "openbao"
	postRetireMaterial, err := serviceaccess.ExistingTLSMaterial(postRetirePolicy, filepath.Join(filepath.Dir(files.Compose), "providers", "openbao", "service-access", "pki"))
	if err != nil {
		return err
	}
	postRetireClient, err := serviceaccess.NewHTTPClient(postRetireMaterial, false)
	if err != nil {
		return err
	}
	postRetireCtx, postRetireCancel := context.WithTimeout(ctx, 15*time.Second)
	defer postRetireCancel()
	if err := serviceaccess.WaitHTTPS(postRetireCtx, postRetireClient, endpoint, "/v1/sys/health"); err != nil {
		return fmt.Errorf("verify OpenBao native HTTPS endpoint after CA retirement: %w", err)
	}
	if _, err := compose.ExecProject(ctx, files.Project, files.Compose, files.Env, "postgres-admin",
		"sh", "-ec",
		"pg_isready -h postgres -p 5432 -U \"$BASEHARBOR_POSTGRES_USER\" -d \"$BASEHARBOR_POSTGRES_DB\""); err != nil {
		return fmt.Errorf("verify control-plane PostgreSQL after CA retirement: %w", err)
	}
	return nil
}

func startControlPlaneRuntime(ctx context.Context, out io.Writer, ports bhruntime.Ports) (bhruntime.RuntimeProvider, bhruntime.Files, error) {
	target, files, err := ensureTargetRuntimeFiles(ctx, ports)
	if err != nil {
		return nil, bhruntime.Files{}, err
	}
	compose, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return nil, bhruntime.Files{}, err
	}
	if err := compose.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return nil, bhruntime.Files{}, err
	}
	startCtx, startCancel := context.WithTimeout(ctx, controlPlaneComposeStartTimeout)
	err = compose.UpProject(startCtx, files.Project, files.Compose, files.Env)
	startCancel()
	if err != nil {
		diagnosticCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if diagnostics, ok := compose.(interface {
			DiagnosticsProject(context.Context, string, string, string) string
		}); ok {
			if details := strings.TrimSpace(diagnostics.DiagnosticsProject(diagnosticCtx, files.Project, files.Compose, files.Env)); details != "" {
				return nil, bhruntime.Files{}, fmt.Errorf("start control plane within %s: %w\n%s", controlPlaneComposeStartTimeout, err, details)
			}
		}
		return nil, bhruntime.Files{}, fmt.Errorf("start control plane within %s: %w", controlPlaneComposeStartTimeout, err)
	}
	return compose, files, nil
}

func startExistingControlPlaneRuntime(ctx context.Context, files bhruntime.Files) (bhruntime.RuntimeProvider, error) {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return nil, err
	}
	compose, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	if err := compose.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return nil, err
	}
	startCtx, startCancel := context.WithTimeout(ctx, controlPlaneComposeStartTimeout)
	err = compose.UpProject(startCtx, files.Project, files.Compose, files.Env)
	startCancel()
	if err != nil {
		diagnosticCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if diagnostics, ok := compose.(interface {
			DiagnosticsProject(context.Context, string, string, string) string
		}); ok {
			if details := strings.TrimSpace(diagnostics.DiagnosticsProject(diagnosticCtx, files.Project, files.Compose, files.Env)); details != "" {
				return nil, fmt.Errorf("restart control plane within %s: %w\n%s", controlPlaneComposeStartTimeout, err, details)
			}
		}
		return nil, fmt.Errorf("restart control plane within %s: %w", controlPlaneComposeStartTimeout, err)
	}
	return compose, nil
}

func verifyExistingControlPlaneAfterStart(ctx context.Context, compose bhruntime.RuntimeProvider, files bhruntime.Files, recoveryFile string, out io.Writer) error {
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
	if err := reconcileControlPlaneServiceAccess(ctx, compose, files, recoveryFile); err != nil {
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
