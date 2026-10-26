package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"io"
)

func restartAfterBackup(ctx context.Context, compose bhruntime.RuntimeProvider, platformFiles bhruntime.Files, resolved resolvedApplication, files application.RuntimeFiles, brokerStopped, workloadStopped, exposureStopped bool) error {
	var result error
	if brokerStopped {
		if err := ensureAndStartRuntimeBroker(ctx, io.Discard, compose, platformFiles, resolved.Manifest, files); err != nil {
			result = errors.Join(result, fmt.Errorf("restart application runtime broker after backup: %w", err))
		}
	}
	if workloadStopped && result == nil {
		if _, err := applyRepositoryWorkload(ctx, io.Discard, compose, resolved, files); err != nil {
			result = errors.Join(result, fmt.Errorf("restart repository workload after backup: %w", err))
		}
	}
	if exposureStopped && result == nil {
		prepared, err := prepareManagedExposure(ctx, compose, resolved)
		if err != nil {
			result = errors.Join(result, fmt.Errorf("prepare managed exposure after backup: %w", err))
		} else if err := convergeManagedExposure(ctx, io.Discard, prepared); err != nil {
			result = errors.Join(result, fmt.Errorf("restart managed exposure after backup: %w", err))
		}
	}
	return result
}
