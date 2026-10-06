package main

import (
	"context"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/preflight"
)

// The repository contract gate is identical for inspection and every lifecycle.
func applicationWorkloadContractCheck(resolved resolvedApplication) preflight.Check {
	return preflight.Check{Name: "application workload", Run: func(context.Context) error { return preflightRepositoryWorkload(resolved) }}
}

func applicationSharedBackendTopologyCheck(resolved resolvedApplication) preflight.Check {
	return preflight.Check{Name: "shared backend topology", Run: func(context.Context) error {
		return application.CheckSharedPostgresTopologyAt(resolved.TargetStateRoot, resolved.Target.Name, resolved.Manifest)
	}}
}
