package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// The ordinary repository entry point uses the same acknowledged application
// lifecycle as app apply. It must not install management before reading intent.
func routeRepositoryRuntimeUp(ctx context.Context, in io.Reader, out, errOut io.Writer, opts runtimeUpOptions) (bool, error) {
	if opts.ControlPlaneOnly {
		return false, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return true, fmt.Errorf("resolve current directory: %w", err)
	}
	found, err := application.HasRepositoryApplication(cwd)
	if err != nil {
		return true, err
	}
	if !found {
		detected, err := detectAppProject(cwd)
		if err != nil {
			return true, err
		}
		if !detectedApplicationProject(detected) {
			return false, nil
		}
	}
	// These select the independently enabled management installation. Reject
	// them before source initialization instead of silently changing its scope.
	if opts.HA || opts.PostgresPort != 0 || opts.OpenBaoPort != 0 || opts.RecoveryFile != "" || opts.TrustHostCA {
		return true, usageError("management Core options require --control-plane-only in an application repository", "Set up or recover the independent Core explicitly with 'baha up --control-plane-only'; then run ordinary 'baha up' for the selected application intent.")
	}
	if !found {
		initialized, err := initializeRepositoryManifestForUp(ctx, in, out, errOut, opts)
		if err != nil || !initialized {
			return true, err
		}
	}
	resolved, err := resolveApplication(ctx, application.DefaultStore(), nil, "up")
	if err != nil {
		return true, err
	}
	if err := preflightRepositoryWorkload(resolved); err != nil {
		return true, fmt.Errorf("application workload preflight failed before provider mutation: %w", err)
	}
	if err := application.CheckSharedPostgresTopologyAt(resolved.TargetStateRoot, resolved.Target.Name, resolved.Manifest); err != nil {
		return true, fmt.Errorf("shared backend topology preflight failed before provider mutation: %w", err)
	}
	if err := ensureRepositoryDeploymentInputsForUp(ctx, in, out, opts); err != nil {
		return true, err
	}
	fmt.Fprintln(out, "Converging selected application capabilities and their genuine dependencies...")
	return true, appApplyCommand(application.DefaultStore()).Run(ctx, nil, out, errOut)
}
