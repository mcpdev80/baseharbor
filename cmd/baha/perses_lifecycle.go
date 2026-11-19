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
//     converge runtime: machine transports (MCP stdio, HTTP/remote) run the
//     shared apply lifecycle immediately; the direct human CLI defers the
//     convergence to a later 'baha app apply' and prints the actionable
//     deferred-apply notice (the repository manifest is the source of truth).
//   - status reports the public three-state contract without a health probe.
//   - remove disables the surface and retires only the owned Perses runtime
//     resources; Prometheus, Loki, Tempo and foreign data are preserved.
//
// Every operation is fail-closed: unauthorized or unresolved-ownership callers
// receive a *application.PersesLifecycleError before any manifest mutation.
// Each command is machine-classified semantic: it shares its typed operation,
// JSON surface and authorization with the MCP transport (0428-084).
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
		Long:    "Selects the optional Perses realization of the observability management-ui surface by setting services.perses in the repository manifest. Machine transports converge runtime through the shared apply lifecycle; the direct human CLI defers it to 'baha app apply' and reports the deferred action. Fail-closed: requires a verified application owner; requires the observability management-ui surface and managed metrics sources (Prometheus). Reports configured_unverified — no Perses health path exists in this Core.",
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
		Long:    "Deselects the optional Perses realization of the observability management-ui surface by clearing services.perses in the repository manifest. Machine transports converge runtime through the shared apply lifecycle; the direct human CLI defers it to 'baha app apply' and reports the deferred action. The backend observability surface (Prometheus) is preserved. Reports not_configured.",
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
			result, err := reportPersesSurfaceStatus(ctx, store, filtered)
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			fmt.Fprintf(out, "Perses state: %s\n", result.State)
			return nil
		},
	}
}

func persesRemoveCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "remove",
		Summary: "Ownership-safe retirement of the owned Perses surface",
		Usage:   "baha app perses remove [NAME] [--json]",
		Long:    "Disables the optional Perses surface and retires only the BaseHarbor-owned Perses runtime resources. Prometheus, Loki, Tempo and foreign data are preserved and never removed. Fail-closed: requires a verified application owner. Machine transports converge runtime through the shared apply lifecycle; the direct human CLI defers it to 'baha app apply' and reports the deferred action. This operation converges the manifest and ownership-safe removal plan.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return runPersesLifecycleCommand(ctx, store, args, out, errOut, application.PersesLifecycleRemove, false)
		},
	}
}

// persesLifecycleResult is the shared JSON/structured surface of every Perses
// lifecycle mutation. DeferredAction carries the actionable deferred-apply
// notice for the direct human CLI (the transport that defers the shared apply
// lifecycle); it is empty on machine transports, which converged through the
// shared apply lifecycle inside the same call.
type persesLifecycleResult struct {
	Action            string `json:"action"`
	State             string `json:"state"`
	RequiresReconcile bool   `json:"requires_reconcile"`
	BackendPreserved  bool   `json:"backend_preserved"`
	Enabled           bool   `json:"enabled"`
	DeferredAction    string `json:"deferred_action,omitempty"`
}

const persesDeferredApplyNotice = "Converging runtime deferred: run 'baha app apply' to reconcile the changed manifest."

type persesStatusResult struct {
	State   string `json:"state"`
	Enabled bool   `json:"enabled"`
	Version string `json:"version"`
}

// persesDirectCLIApplyDeferred reports whether the shared apply lifecycle after
// a manifest change is deferred to a later human 'baha app apply'. Only the
// direct human CLI invocation ("cli") defers; it must print the actionable
// deferred-apply notice. MCP stdio, HTTP, remote and every other machine
// transport leave this false and run the shared apply lifecycle immediately,
// because a non-interactive client has no later human apply. main.go tags
// direct CLI runs with withLocalInvocation(ctx, "cli") and the MCP stdio
// transport with "stdio"; every other machine path leaves the transport empty.
var persesDirectCLIApplyDeferred = func(ctx context.Context) bool {
	return cliInvocationTransport(ctx) == "cli"
}

// persesApplyLifecycleFunc is the shared apply lifecycle seam the Perses
// lifecycle converges through. It is a package-level variable so the
// transport/action matrix tests can observe the machine transports'
// convergence hermetically (the mission forbids containers on the coordinator
// host); production wires the real executeApplicationApplyLifecycle.
var persesApplyLifecycleFunc = func(ctx context.Context, store application.Store, args []string, out, errOut io.Writer) error {
	return executeApplicationApplyLifecycle(ctx, store, args, out, errOut)
}

// persesLifecycleStatusAction names the read action behind 'baha app perses
// status' and its typed MCP operation (app.perses.status); the machine
// contract registers no mutation for it.
const persesLifecycleStatusAction application.PersesLifecycleAction = "status"

// persesLifecycleOperationID binds each lifecycle action to its registered
// typed machine operation (internal/machine/contract.go), the same identity
// cliMachineOperations maps the CLI command form to. The bare "perses.<action>"
// audit label was never a registry id: the unregistered id made every
// authorization fail with "Unknown BaseHarbor machine operation".
func persesLifecycleOperationID(action application.PersesLifecycleAction) string {
	return "app.perses." + string(action)
}

// executePersesLifecycle is the single mutation seam the CLI commands and the
// typed MCP operations share: fail-closed ownership guard, plan resolution and
// the repository manifest as the source of truth (JSON parity per the
// cli-json-mcp-parity contract).
func executePersesLifecycle(ctx context.Context, store application.Store, args []string, action application.PersesLifecycleAction, setPerses bool, out, errOut io.Writer) (persesLifecycleResult, error) {
	resolved, err := resolveApplication(ctx, store, args, "perses "+string(action))
	if err != nil {
		return persesLifecycleResult{}, err
	}
	// Fail-closed ownership/authorization guard before any manifest mutation,
	// under the registered typed operation id the machine-coverage map binds
	// for this command ("app.perses.<action>").
	if err := authorizeApplicationOperation(ctx, persesLifecycleOperationID(action), resolved); err != nil {
		return persesLifecycleResult{}, err
	}
	plan, err := application.ResolvePersesLifecycle(resolved.Manifest, action, setPerses, true, true)
	if err != nil {
		return persesLifecycleResult{}, err
	}
	if err := persistRepositoryPersesLifecycle(ctx, resolved, plan); err != nil {
		return persesLifecycleResult{}, err
	}
	if plan.RequiresReconcile && !persesDirectCLIApplyDeferred(ctx) {
		if err := persesApplyLifecycleFunc(ctx, store, args, out, errOut); err != nil {
			return persesLifecycleResult{}, err
		}
	}
	result := persesLifecycleResult{
		Action:            string(action),
		State:             application.PersesLifecycleStatus(plan.ManifestAfter),
		RequiresReconcile: plan.RequiresReconcile,
		BackendPreserved:  plan.BackendPreserved,
		Enabled:           application.PersesLifecycleEnabled(plan.ManifestAfter),
	}
	if plan.RequiresReconcile && persesDirectCLIApplyDeferred(ctx) {
		// The direct human CLI defers the shared apply lifecycle; the deferred
		// action must then be explicit in the result (JSON parity and the
		// human notice below), so a changed manifest never reports silent
		// success.
		result.DeferredAction = persesDeferredApplyNotice
	}
	return result, nil
}

// reportPersesSurfaceStatus is the shared read seam behind 'baha app perses
// status' and the typed MCP operation: the public three-state contract from
// manifest truth, never a health probe.
func reportPersesSurfaceStatus(ctx context.Context, store application.Store, args []string) (persesStatusResult, error) {
	resolved, err := resolveApplication(ctx, store, args, "perses status")
	if err != nil {
		return persesStatusResult{}, err
	}
	if err := authorizeApplicationOperation(ctx, persesLifecycleOperationID(persesLifecycleStatusAction), resolved); err != nil {
		return persesStatusResult{}, err
	}
	return persesStatusResult{
		State:   application.PersesLifecycleStatus(resolved.Manifest),
		Enabled: application.PersesLifecycleEnabled(resolved.Manifest),
		Version: capability.PersesDefaultRelease.Version,
	}, nil
}

func runPersesLifecycleCommand(ctx context.Context, store application.Store, args []string, out, errOut io.Writer, action application.PersesLifecycleAction, setPerses bool) error {
	filtered, format, err := parseReadOutputArgs(args, "perses "+string(action))
	if err != nil {
		return err
	}
	changed, err := currentPersesSurfaceChanged(ctx, store, filtered, action, setPerses)
	if err != nil {
		return err
	}
	result, err := executePersesLifecycle(ctx, store, filtered, action, setPerses, out, errOut)
	if err != nil {
		if lifecycleErr := new(application.PersesLifecycleError); errors.As(err, &lifecycleErr) {
			fmt.Fprintf(out, "FAILED: %s\n", err)
		}
		return err
	}
	if !changed {
		fmt.Fprintf(out, "No change: %s already at the target state.\n", action)
	} else if result.DeferredAction != "" {
		// The direct human CLI deferred the shared apply lifecycle: every
		// changed mutation (enable, disable, remove) must print the actionable
		// deferred-apply notice here, in every output format.
		fmt.Fprintf(out, "%s\n", result.DeferredAction)
	}
	if format == outputJSON {
		return writeJSON(out, result)
	}
	return nil
}

// currentPersesSurfaceChanged reports whether the surface selection actually
// changes under this action, so the human path keeps its "No change" notice.
func currentPersesSurfaceChanged(ctx context.Context, store application.Store, args []string, action application.PersesLifecycleAction, setPerses bool) (bool, error) {
	resolved, err := resolveApplication(ctx, store, args, "perses "+string(action))
	if err != nil {
		return false, err
	}
	return application.PersesLifecycleEnabled(resolved.Manifest) != setPerses, nil
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

// cliInvocationTransport reports the invocation transport tag main.go and the
// MCP servers stamp into the context ("cli", "stdio", or "" for HTTP/remote and
// plain test invocations). Shared machine seams use it to tell direct human CLI
// runs from non-interactive machine runs.
func cliInvocationTransport(ctx context.Context) string {
	transport, _ := ctx.Value(localInvocationKey{}).(string)
	return transport
}
