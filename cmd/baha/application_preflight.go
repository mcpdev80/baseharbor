package main

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/preflight"
)

// The repository contract gate is identical for inspection and every lifecycle.
func applicationWorkloadContractCheck(resolved resolvedApplication) preflight.Check {
	return preflight.Check{Name: "application workload", Run: func(context.Context) error { return preflightRepositoryWorkload(resolved) }}
}
