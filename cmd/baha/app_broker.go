package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
	"github.com/mcpdev80/baseharbor/internal/runtimeexecutor"
)

func ensureAndStartRuntimeBroker(ctx context.Context, progress io.Writer, compose bhruntime.Compose, platformFiles bhruntime.Files, m application.Manifest, files application.RuntimeFiles) error {
	if !application.RequiresRuntimeBroker(m) {
		return nil
	}
	if err := ensureAndStartRuntimeProviderExecutor(ctx, progress, compose, platformFiles, m); err != nil {
		return err
	}
	identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
	mtlsFiles, identityChanged, err := openbao.EnsureRuntimeMTLSIdentity(ctx, compose, platformFiles, identity, files)
	if err != nil {
		return fmt.Errorf("converge runtime mTLS identity: %w", err)
	}
	brokerFiles, err := runtimebroker.Ensure(m, files, mtlsFiles)
	if err != nil {
		return fmt.Errorf("materialize application runtime broker: %w", err)
	}
	project := runtimebroker.ProjectName(m)
	if err := compose.ConfigProject(ctx, project, brokerFiles.Compose, files.Env); err != nil {
		return fmt.Errorf("validate application runtime broker: %w", err)
	}
	if identityChanged {
		// Runtime identity files are installed atomically. Existing containers can
		// otherwise retain the old bind-mounted inode, so an actual rotation must
		// recreate the broker before readiness is evaluated.
		if err := compose.DownProject(ctx, project, brokerFiles.Compose, files.Env); err != nil {
			return fmt.Errorf("restart application runtime broker after mTLS rotation: %w", err)
		}
	}
	if err := compose.UpProjectProgress(ctx, project, brokerFiles.Compose, files.Env, func(detail string) {
		cli.ReportActivityDetail(progress, detail)
	}); err != nil {
		return fmt.Errorf("start application runtime broker: %w", err)
	}
	verifyCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var verifyErr error
	for verifyCtx.Err() == nil {
		verifyErr = verifyRuntimeBrokerRunning(verifyCtx, compose, m, files)
		if verifyErr == nil {
			return nil
		}
		select {
		case <-verifyCtx.Done():
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("application runtime broker readiness failed: %w", verifyErr)
}

func ensureAndStartRuntimeProviderExecutor(ctx context.Context, progress io.Writer, compose bhruntime.Compose, platformFiles bhruntime.Files, m application.Manifest) error {
	if !requiresRuntimeObjectStorageExecutor(m) {
		return nil
	}
	if platformFiles.Compose == "" || platformFiles.Env == "" {
		return errors.New("BaseHarbor control-plane runtime is required for runtime provider executor PKI")
	}
	_, _, adminCredentialsPath, err := objectstorage.EnsureSharedProvider(ctx, compose)
	if err != nil {
		return fmt.Errorf("converge runtime object-storage provider: %w", err)
	}
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return fmt.Errorf("resolve BaseHarbor data directory for runtime executor: %w", err)
	}
	identityDir := filepath.Join(dataDir, "runtime-executor", "identity")
	identity, identityChanged, err := openbao.EnsureRuntimeExecutorMTLSIdentity(ctx, compose, platformFiles, identityDir)
	if err != nil {
		return fmt.Errorf("converge runtime executor mTLS identity: %w", err)
	}
	executorFiles, err := runtimeexecutor.EnsureFiles(dataDir, identity, adminCredentialsPath)
	if err != nil {
		return fmt.Errorf("materialize runtime provider executor: %w", err)
	}
	if err := compose.ConfigProject(ctx, runtimeexecutor.ProjectName, executorFiles.Compose, executorFiles.Env); err != nil {
		return fmt.Errorf("validate runtime provider executor: %w", err)
	}
	if identityChanged {
		if err := compose.DownProject(ctx, runtimeexecutor.ProjectName, executorFiles.Compose, executorFiles.Env); err != nil {
			return fmt.Errorf("restart runtime provider executor after mTLS rotation: %w", err)
		}
	}
	if err := compose.UpProjectProgress(ctx, runtimeexecutor.ProjectName, executorFiles.Compose, executorFiles.Env, func(detail string) {
		cli.ReportActivityDetail(progress, detail)
	}); err != nil {
		return fmt.Errorf("start runtime provider executor: %w", err)
	}
	services, err := compose.RunningServicesProject(ctx, runtimeexecutor.ProjectName, executorFiles.Compose, executorFiles.Env)
	if err != nil {
		return fmt.Errorf("inspect runtime provider executor: %w", err)
	}
	if len(services) != 1 || services[0] != runtimeexecutor.ServiceName {
		return errors.New("runtime provider executor is not running")
	}
	return nil
}

func waitRuntimeBrokerReady(ctx context.Context, compose bhruntime.Compose, m application.Manifest, files application.RuntimeFiles, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var lastErr error
	for {
		lastErr = verifyRuntimeBrokerRunning(waitCtx, compose, m, files)
		if lastErr == nil {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("application runtime broker did not become ready within %s: %w", timeout, lastErr)
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func verifyRuntimeBrokerRunning(ctx context.Context, compose bhruntime.Compose, m application.Manifest, files application.RuntimeFiles) error {
	if !application.RequiresRuntimeBroker(m) {
		return nil
	}
	brokerFiles, err := runtimebroker.Existing(files)
	if err != nil {
		return fmt.Errorf("application runtime broker state is missing: %w", err)
	}
	project := runtimebroker.ProjectName(m)
	services, err := compose.RunningServicesProject(ctx, project, brokerFiles.Compose, files.Env)
	if err != nil {
		return fmt.Errorf("inspect application runtime broker: %w", err)
	}
	if len(services) != 1 || services[0] != runtimebroker.ServiceName {
		return errors.New("application runtime broker is not running")
	}
	out, err := compose.ExecProject(ctx, project, brokerFiles.Compose, files.Env, runtimebroker.ServiceName,
		"curl", "--fail", "--silent", "--show-error",
		"--resolve", "baseharbor-runtime:8443:127.0.0.1",
		"--cacert", "/run/baseharbor/identity/ca.pem",
		"--cert", "/run/secrets/probe-client-cert",
		"--key", "/run/secrets/probe-client-key",
		runtimebroker.RuntimeURL+"/readyz",
	)
	if err != nil {
		return fmt.Errorf("application runtime broker mTLS readiness probe failed: %w", err)
	}
	if !strings.Contains(out, `"status":"ready"`) {
		return errors.New("application runtime broker readiness response is invalid")
	}
	return nil
}

func printRuntimeBrokerDocs(out io.Writer, files application.RuntimeFiles) {
	brokerFiles, err := runtimebroker.Existing(files)
	if err != nil || strings.TrimSpace(brokerFiles.DocsURL) == "" {
		return
	}
	fmt.Fprintf(out, "[INFO] runtime-broker    Swagger/OpenAPI: %s\n", brokerFiles.DocsURL)
}

func stopRuntimeBroker(ctx context.Context, compose bhruntime.Compose, m application.Manifest, files application.RuntimeFiles) error {
	if !application.RequiresRuntimeBroker(m) {
		return nil
	}
	brokerFiles, err := runtimebroker.Existing(files)
	if err != nil {
		return fmt.Errorf("application runtime broker state is missing: %w", err)
	}
	if err := compose.DownProject(ctx, runtimebroker.ProjectName(m), brokerFiles.Compose, files.Env); err != nil {
		return fmt.Errorf("stop application runtime broker: %w", err)
	}
	services, err := compose.RunningServicesProject(ctx, runtimebroker.ProjectName(m), brokerFiles.Compose, files.Env)
	if err != nil {
		return fmt.Errorf("verify application runtime broker stop: %w", err)
	}
	if len(services) != 0 {
		return errors.New("verify application runtime broker stop: broker is still running")
	}
	return nil
}
