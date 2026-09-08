package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
)

func ensureAndStartRuntimeBroker(ctx context.Context, compose bhruntime.Compose, platformFiles bhruntime.Files, m application.Manifest, files application.RuntimeFiles) error {
	if !m.Services.Secrets {
		return nil
	}
	identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
	mtlsFiles, err := openbao.EnsureRuntimeMTLSIdentity(ctx, compose, platformFiles, identity, files)
	if err != nil {
		return fmt.Errorf("converge runtime mTLS identity: %w", err)
	}
	brokerFiles, err := runtimebroker.Ensure(m, files, mtlsFiles)
	if err != nil {
		return fmt.Errorf("materialize runtime secret broker: %w", err)
	}
	project := runtimebroker.ProjectName(m)
	if err := compose.ConfigProject(ctx, project, brokerFiles.Compose, files.Env); err != nil {
		return fmt.Errorf("validate runtime secret broker: %w", err)
	}
	if err := compose.UpProject(ctx, project, brokerFiles.Compose, files.Env); err != nil {
		return fmt.Errorf("start runtime secret broker: %w", err)
	}
	return verifyRuntimeBrokerRunning(ctx, compose, m, files)
}

func verifyRuntimeBrokerRunning(ctx context.Context, compose bhruntime.Compose, m application.Manifest, files application.RuntimeFiles) error {
	if !m.Services.Secrets {
		return nil
	}
	brokerFiles, err := runtimebroker.Existing(files)
	if err != nil {
		return fmt.Errorf("runtime secret broker state is missing: %w", err)
	}
	services, err := compose.RunningServicesProject(ctx, runtimebroker.ProjectName(m), brokerFiles.Compose, files.Env)
	if err != nil {
		return fmt.Errorf("inspect runtime secret broker: %w", err)
	}
	if len(services) != 1 || services[0] != runtimebroker.ServiceName {
		return errors.New("runtime secret broker is not running")
	}
	return nil
}

func stopRuntimeBroker(ctx context.Context, compose bhruntime.Compose, m application.Manifest, files application.RuntimeFiles) error {
	if !m.Services.Secrets {
		return nil
	}
	brokerFiles, err := runtimebroker.Existing(files)
	if err != nil {
		return fmt.Errorf("runtime secret broker state is missing: %w", err)
	}
	if err := compose.DownProject(ctx, runtimebroker.ProjectName(m), brokerFiles.Compose, files.Env); err != nil {
		return fmt.Errorf("stop runtime secret broker: %w", err)
	}
	services, err := compose.RunningServicesProject(ctx, runtimebroker.ProjectName(m), brokerFiles.Compose, files.Env)
	if err != nil {
		return fmt.Errorf("verify runtime secret broker stop: %w", err)
	}
	if len(services) != 0 {
		return errors.New("verify runtime secret broker stop: broker is still running")
	}
	return nil
}
