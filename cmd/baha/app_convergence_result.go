package main

import (
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

func observedApplicationState(status application.StatusResult) string {
	if status.Ready {
		return "ready"
	}
	return status.State
}

func reportApplicationConvergence(term *cli.Terminal, status application.StatusResult) error {
	if status.Ready {
		term.Success("READY", "application and requested infrastructure verified")
		return nil
	}
	if status.State == "running" && status.HasUnverified() && !status.HasFailures() {
		term.Result("RUNNING", "application", "workload is running; readiness remains unverified")
		return nil
	}
	term.Result("DEGRADED", "application", "one or more readiness checks failed")
	return fmt.Errorf("application convergence did not establish readiness; run 'baha doctor' for failed checks")
}
