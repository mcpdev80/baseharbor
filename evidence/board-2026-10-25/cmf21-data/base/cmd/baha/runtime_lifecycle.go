package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func runtimeDestroy(parent context.Context, args []string, out io.Writer) error {
	confirmed := false
	for _, arg := range args {
		switch arg {
		case "--yes":
			confirmed = true
		case "--delete-data":
			parent = withSelectedCoreDataDeletion(parent, true)
		default:
			return usageError("unknown argument "+arg, "Usage: baha destroy [--yes] [--delete-data]")
		}
	}

	return destroyControlPlane(parent, confirmed, out)
}
func destroyControlPlane(parent context.Context, confirmed bool, out io.Writer) error {
	return destroySelectedControlPlane(parent, confirmed, out)
}

func runtimeStatus(parent context.Context, out io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	result, err := inspectControlPlane(ctx)
	renderDockerEngine(out, result.DockerEngine)
	if err != nil {
		return err
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "BaseHarbor · %s\n\nTarget\n  EFFECTIVE  %s\n  Runtime    %s\n  Access     %s (%s)\n", target.Name, target.Name, target.RuntimeProvider, target.AccessReference, target.AccessProvider)
	if target.Scope != "" {
		fmt.Fprintf(out, "  Scope      %s\n", target.Scope)
	}
	fmt.Fprintln(out, "\nControl Plane")
	switch result.State {
	case "not_deployed":
		fmt.Fprintln(out, "  NOT DEPLOYED")
		return nil
	case "stopped":
		fmt.Fprintln(out, "  STOPPED")
	default:
		if len(result.Checks) == 0 {
			fmt.Fprintf(out, "  RUNNING    %d service(s)\n", len(result.Running))
		}
		for _, check := range result.Checks {
			state := "READY"
			if !check.Ready {
				state = "FAILED"
			}
			fmt.Fprintf(out, "  %-9s %s\n", state, check.Name)
		}
		if !result.Ready {
			return errors.New("runtime is running but not ready")
		}
		state := "SATISFIED"
		if !result.AvailabilitySatisfied {
			state = "UNSATISFIED"
		}
		fmt.Fprintf(out, "  %-9s availability · %s\n", state, result.AvailabilityDetail)
	}
	records, warnings, listErr := deployment.ListDeploymentsForDisplay(target.Name)
	fmt.Fprintln(out, "\nApplications")
	if listErr != nil {
		fmt.Fprintf(out, "  UNKNOWN    %v\n", listErr)
	} else {
		fmt.Fprintf(out, "  %d registered deployment(s)\n", len(records))
		for _, warning := range warnings {
			fmt.Fprintf(out, "  WARN       %v\n", warning)
		}
	}
	return nil
}

func initConfig(out io.Writer) error {
	fmt.Fprintln(out, "baha init no longer creates a global baseharbor.yaml.")
	fmt.Fprintln(out, "Application intent belongs in the repository manifest: run 'baha app init'.")
	fmt.Fprintln(out, "Deployment runtime/target selection belongs in Target configuration: run 'baha target create --help'.")
	return nil
}
