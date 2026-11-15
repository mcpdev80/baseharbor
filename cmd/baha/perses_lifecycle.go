package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

// persesLifecycleCommand wires the accepted CLI/domain operation for the optional
// Perses observability management surface: enable, disable, status and an
// ownership-safe remove. It is bounded to the manifest contract and the existing
// application apply reconciliation:
//
//   - enable/disable edit the repository manifest services.perses field and then
//     converge runtime with the existing 'baha app apply' path (the repository
//     manifest is the source of truth).
//   - status reports the public three-state contract without a health probe.
//   - remove disables the surface and retires only the owned Perses runtime
//     resources; Prometheus, Loki, Tempo and foreign data are preserved.
//
// Every operation is fail-closed: unauthorized or unresolved-ownership callers
// receive a *application.PersesLifecycleError before any manifest mutation.
func persesLifecycleCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "perses",
		Summary: "Guide enable/status/disable and ownership-safe removal of the optional Perses surface",
		Usage:   "baha app perses <command> [NAME] [--json]",
		Long:    "Guided lifecycle for the optional Perses observability management surface. Enable selects the optional Perses realization of the observability management-ui surface; disable deselects it (the surface returns to not_configured); status reports the public three-state contract; remove disables the surface and retires only the BaseHarbor-owned Perses runtime resources, preserving Prometheus, Loki, Tempo and foreign data. Fail-closed: an unauthorized or unresolved-ownership caller fails before any manifest mutation. Perses remains optional and never forces a new application capability.",
		Children: []*cli.Command{
			persesEnableCommand(store),
			persesDisableCommand(store),
			persesStatusCommand(store),
			persesRemoveCommand(store),
		},
	}
}

func persesEnableCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "enable",
		Summary: "Select the optional Perses management-ui surface",
		Usage:   "baha app perses enable [NAME] [--json]",
		Long:    "Selects the optional Perses realization of the observability management-ui surface by setting services.perses in the repository manifest and converging runtime with 'baha app apply'. Fail-closed: requires a verified application owner; requires the observability management-ui surface and managed metrics sources (Prometheus). Reports configured_unverified — no Perses health path exists in this Core.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return runPersesLifecycleCommand(ctx, store, args, out, errOut, application.PersesLifecycleEnable, true)
		},
	}
}

func persesDisableCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "disable",
		Summary: "Deselect the optional Perses management-ui surface",
		Usage:   "baha app perses disable [NAME] [--json]",
		Long:    "Deselects the optional Perses realization of the observability management-ui surface by clearing services.perses in the repository manifest and converging runtime with 'baha app apply'. The backend observability surface (Prometheus) is preserved. Reports not_configured.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return runPersesLifecycleCommand(ctx, store, args, out, errOut, application.PersesLifecycleDisable, false)
		},
	}
}

func persesStatusCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "status",
		Summary: "Report the Perses three-state contract",
		Usage:   "baha app perses status [NAME] [--json]",
		Long:    "Reports the public three-state contract for the optional Perses surface (not_configured | configured_unverified). It never reports verified: this Core ships no Perses health path.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "perses status")
			if err != nil {
				return err
			}
			resolved, err := resolveApplication(ctx, store, filtered, "perses status")
			if err != nil {
				return err
			}
			if err := authorizeApplicationOperation(ctx, "perses.status", resolved); err != nil {
				return err
			}
			status := application.PersesLifecycleStatus(resolved.Manifest)
			result := persesStatusResult{State: status, Enabled: application.PersesLifecycleEnabled(resolved.Manifest), Version: capability.PersesDefaultRelease.Version}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			fmt.Fprintf(out, "Perses state: %s\n", status)
			return nil
		},
	}
}

func persesRemoveCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "remove",
		Summary: "Ownership-safe retirement of the owned Perses surface",
		Usage:   "baha app perses remove [NAME] [--json]",
		Long:    "Disables the optional Perses surface and retires only the BaseHarbor-owned Perses runtime resources. Prometheus, Loki, Tempo and foreign data are preserved and never removed. Fail-closed: requires a verified application owner. Runtime removal is deferred to the release window and reported as STATE; this operation converges the manifest and ownership-safe removal plan.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return runPersesLifecycleCommand(ctx, store, args, out, errOut, application.PersesLifecycleRemove, false)
		},
	}
}

type persesLifecycleResult struct {
	Action            string `json:"action"`
	State             string `json:"state"`
	RequiresReconcile bool   `json:"requires_reconcile"`
	BackendPreserved  bool   `json:"backend_preserved"`
	Enabled           bool   `json:"enabled"`
}

type persesStatusResult struct {
	State   string `json:"state"`
	Enabled bool   `json:"enabled"`
	Version string `json:"version"`
}

func runPersesLifecycleCommand(ctx context.Context, store application.Store, args []string, out, errOut io.Writer, action application.PersesLifecycleAction, setPerses bool) error {
	filtered, format, err := parseReadOutputArgs(args, "perses "+string(action))
	if err != nil {
		return err
	}
	resolved, err := resolveApplication(ctx, store, filtered, "perses "+string(action))
	if err != nil {
		return err
	}
	// Fail-closed ownership/authorization guard before any manifest mutation.
	if err := authorizeApplicationOperation(ctx, "perses."+string(action), resolved); err != nil {
		return err
	}

	plan, err := application.ResolvePersesLifecycle(resolved.Manifest, action, setPerses, true, true)
	if err != nil {
		if lifecycErr := new(application.PersesLifecycleError); errors.As(err, &lifecycErr) {
			fmt.Fprintf(out, "FAILED: %s\n", err)
			return err
		}
		return err
	}
	if !plan.RequiresReconcile {
		fmt.Fprintf(out, "No change: %s already at the target state.\n", action)
	}

	// Persist the repository manifest as the source of truth and converge
	// runtime with the existing apply path.
	if err := persistRepositoryPersesLifecycle(ctx, resolved, plan); err != nil {
		return err
	}
	if plan.RequiresReconcile {
		fmt.Fprintf(out, "Converging runtime: run 'baha app apply' (deferred to release window under current gate scope)\n")
	}

	result := persesLifecycleResult{
		Action:            string(action),
		State:             application.PersesLifecycleStatus(plan.ManifestAfter),
		RequiresReconcile: plan.RequiresReconcile,
		BackendPreserved:  plan.BackendPreserved,
		Enabled:           application.PersesLifecycleEnabled(plan.ManifestAfter),
	}
	if format == outputJSON {
		return writeJSON(out, result)
	}
	return nil
}

// persistRepositoryPersesLifecycle writes the repository manifest as the source
// of truth, preserving the existing application identity and all other fields,
// so the repository manifest reflects the converged services.perses selection.
func persistRepositoryPersesLifecycle(ctx context.Context, resolved resolvedApplication, plan application.PersesLifecyclePlan) error {
	if strings.TrimSpace(resolved.ManifestPath) == "" {
		return errors.New("perses lifecycle requires a repository-owned baseharbor.yaml")
	}
	manifest := plan.ManifestAfter
	manifest.ApplicationID = resolved.Manifest.ApplicationID
	if err := manifest.Validate(); err != nil {
		return err
	}
	path := resolved.ManifestPath
	if err := os.WriteFile(path, []byte(manifest.AuthoredYAML()), 0644); err != nil {
		return fmt.Errorf("write repository manifest for perses lifecycle: %w", err)
	}
	_ = ctx
	return nil
}
