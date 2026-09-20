package main

import (
	"context"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
	tracesprovider "github.com/mcpdev80/baseharbor/internal/traces"
)

type managedTelemetryExecution struct {
	execution *capability.Execution
	driver    *telemetry.Driver
	manifest  application.Manifest
}

func prepareManagedTelemetry(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication, traces *managedTracesExecution) (*managedTelemetryExecution, error) {
	m := resolved.Manifest
	if !application.HasOTLPTelemetry(m) {
		return nil, nil
	}
	files := application.RuntimeFilesFor(resolved.Store, m)
	driver := telemetry.NewDriver(compose, m, files)
	if traces != nil && traces.enabled {
		driver.SetTraceBackend("http://tempo:4318", traces.placement.Network)
	} else if enabled, policyErr := application.TracesCollectionEnabled(m); policyErr != nil {
		return nil, policyErr
	} else if enabled {
		if _, placement, stateErr := tracesprovider.ExistingProviderFiles(m); stateErr == nil {
			driver.SetTraceBackend("http://tempo:4318", placement.Network)
		}
	}
	request := capability.Request{
		Requirement: capability.Requirement{Kind: capability.TelemetryOTLP, Name: "default"},
		Workload:    "application/" + m.Name,
		TelemetryOTLP: &capability.OTLPTelemetryBinding{
			Direction: "export",
			Protocol:  "http/protobuf",
			Signals:   append([]string(nil), m.Telemetry.OTLP.Signals...),
		},
		Driver: driver,
	}
	execution, _, err := capability.Prepare(ctx, m.Name, []capability.Request{request})
	if err != nil {
		return nil, err
	}
	return &managedTelemetryExecution{execution: execution, driver: driver, manifest: m}, nil
}

func convergeManagedTelemetry(ctx context.Context, out io.Writer, prepared *managedTelemetryExecution) error {
	if prepared == nil {
		return nil
	}
	if _, err := prepared.execution.ProvisionAndBind(ctx); err != nil {
		return err
	}
	if _, err := prepared.execution.Verify(ctx); err != nil {
		return err
	}
	fmt.Fprintf(out, "[OK] telemetry          OTLP export verified for %s\n", prepared.manifest.Name)
	return nil
}
