package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
)

// A retired broker has no loaded Quadlet unit. Use the existing selected-service
// stop authority: inspect canonical ownership and actual containers, stop only
// the present broker, and verify absence/stopped state while retaining siblings.
func stopRestoreRuntimeBroker(ctx context.Context, provider bhruntime.RuntimeProvider, m application.Manifest, files application.RuntimeFiles) error {
	native, ok := provider.(bhruntime.RuntimeServiceStopProvider)
	if !ok {
		return fmt.Errorf("restore broker stop requires owned selected-service inspection")
	}
	broker, err := runtimebroker.Existing(files)
	if err != nil {
		return fmt.Errorf("restore broker owner: %w", err)
	}
	protected, err := runtimeMutationOwnerBytes(broker.Compose, false)
	if err != nil {
		return err
	}
	zeroBytes(protected)
	expected, err := runtimebroker.OwnedRuntimeResources(m, files)
	if err != nil {
		return err
	}
	environment, err := bhruntime.RuntimeEnvironment(bhruntime.Files{Env: files.Env})
	if err != nil {
		return err
	}
	_, err = bhruntime.StopOwnedProjectServices(ctx, native, runtimebroker.ProjectNameForRuntime(m, files), filepath.Dir(broker.Compose), environment, expected, []string{runtimebroker.ServiceName}, broker.Compose)
	return err
}
