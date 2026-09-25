package main

import (
	"context"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

func appUpCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "up",
		Summary: "Start an existing application runtime and verify readiness",
		Usage:   "baha app up [NAME]",
		Long:    "Starts a previously materialized BaseHarbor application runtime using its existing runtime definition, credentials and persistent data; required application secrets are verified before workload start and missing values fail closed. Repository workloads are started after their BaseHarbor backend and per-application secret broker are ready. Workload-only applications skip the empty managed-runtime start and resume their repository Compose workload directly. Without NAME it resolves the nearest repository baseharbor.yaml.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return executeApplicationUpLifecycle(ctx, store, args, out, errOut)
		},
	}
}
