package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/exposure"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type managedExposureExecution struct {
	execution *capability.Execution
	driver    *exposure.Driver
}

func prepareManagedExposure(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication) (*managedExposureExecution, error) {
	m := resolved.Manifest
	if len(m.Exposures) == 0 {
		return nil, nil
	}
	if !resolved.FromRepository {
		return nil, errors.New("managed HTTP exposure requires a repository-owned baseharbor.yaml")
	}
	state, err := loadRepositoryInitState(resolved.RepositoryRoot)
	if err != nil {
		return nil, fmt.Errorf("load repository deployment state for managed exposure: %w", err)
	}
	runtime := application.RuntimeFilesFor(resolved.Store, m)
	driver := exposure.NewDriver(compose, m, runtime, exposure.Deployment{
		Hostname: state.Hostname,
		TLSMode:  state.TLSMode,
		TLSDir:   state.TLSDir,
	})
	requests := make([]capability.Request, 0, len(m.Exposures))
	for _, route := range m.Exposures {
		requests = append(requests, capability.Request{
			Requirement: capability.Requirement{Kind: capability.ExposureHTTP, Name: route.Name},
			Workload:    "service/" + route.Service,
			Driver:      driver,
		})
	}
	execution, _, err := capability.Prepare(ctx, m.Name, requests)
	if err != nil {
		return nil, err
	}
	return &managedExposureExecution{execution: execution, driver: driver}, nil
}

func convergeManagedExposure(ctx context.Context, out io.Writer, prepared *managedExposureExecution) error {
	if prepared == nil {
		return nil
	}
	if _, err := prepared.execution.ProvisionAndBind(ctx); err != nil {
		return err
	}
	if _, err := prepared.execution.Verify(ctx); err != nil {
		return err
	}
	state := prepared.driver.State()
	for _, route := range state.Routes {
		fmt.Fprintf(out, "[OK] managed-exposure  %s %s://%s:%d -> %s:%d\n",
			route.Name, route.Protocol, state.Host, route.PublishedPort, route.Service, route.TargetPort)
	}
	return nil
}

func inspectManagedExposure(ctx context.Context, compose bhruntime.Compose, m application.Manifest, files application.RuntimeFiles) ([]string, error) {
	if len(m.Exposures) == 0 {
		return nil, nil
	}
	state, statuses, err := exposure.Inspect(ctx, compose, files)
	lines := make([]string, 0, len(statuses))
	for _, status := range statuses {
		prefix := "[OK]"
		if !status.Ready {
			prefix = "[FAIL]"
		}
		lines = append(lines, fmt.Sprintf("%s managed-exposure/%-10s %s://%s:%d %s",
			prefix, exposureNameForServicePort(state, status.Service, status.Port),
			status.Scheme, status.Host, status.Port, status.Detail))
	}
	return lines, err
}

func exposureNameForServicePort(state exposure.State, service string, publishedPort int) string {
	for _, route := range state.Routes {
		if route.Service == service && route.PublishedPort == publishedPort {
			return route.Name
		}
	}
	return service
}

func stopManagedExposure(ctx context.Context, compose bhruntime.Compose, m application.Manifest, files application.RuntimeFiles) error {
	if len(m.Exposures) == 0 {
		return nil
	}
	if err := exposure.Stop(ctx, compose, files); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stop managed HTTP exposure provider: %w", err)
	}
	return nil
}

func destroyManagedExposure(ctx context.Context, compose bhruntime.Compose, m application.Manifest, files application.RuntimeFiles) error {
	if len(m.Exposures) == 0 {
		return nil
	}
	if err := exposure.Destroy(ctx, compose, files); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("destroy managed HTTP exposure provider: %w", err)
	}
	return nil
}
