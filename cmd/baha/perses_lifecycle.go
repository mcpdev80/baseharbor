package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
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
//   - status reports the Perses surface and the applied alert evaluator's live
//     rule-health verdict.
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
		Long:    "Selects the optional Perses realization of the observability management-ui surface by setting services.perses in the repository manifest. Machine transports converge runtime through the shared apply lifecycle; the direct human CLI defers it to 'baha app apply' and reports the deferred action. Fail-closed: requires a verified application owner and the observability management-ui surface with managed Prometheus metrics sources.",
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
		Summary: "Report Perses state and live alert-rule health",
		Usage:   "baha app perses status [NAME] [--json]",
		Long:    "Reports the optional Perses surface state and probes the applied Prometheus alert rule through the Target-scoped health verifier.",
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
			renderPersesBackendWitness(out, result.BackendWitness)
			if result.AlertEvaluator != nil {
				fmt.Fprintf(out, "  Alert evaluator: %s\n", result.AlertEvaluator.State)
				if result.AlertEvaluator.Detail != "" {
					fmt.Fprintf(out, "    %s\n", result.AlertEvaluator.Detail)
				}
			}
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
	// BackendWitness names, per observability backend, the manifest-only
	// three-state witness (configured / not-configured) with its source-id, so
	// the operator sees which source is declared rather than a blanket
	// "unavailable". Shared with app_status_result.go (persesBackendSignalWitness
	// / persesBackendSignalWitnessFromManifest). Never asserts a verified backend.
	BackendWitness []persesBackendSignalVerdict `json:"backend_witness,omitempty"`
	// AlertEvaluator carries the Target-scoped live rule-health result,
	// separately from the backend signal witness.
	AlertEvaluator *application.StatusCheck `json:"alert_evaluator,omitempty"`
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

// persesAlertEvaluatorStateNotConfigured is the user-visible state when the
// Perses alert evaluator is not selected in the manifest, and
// persesAlertEvaluatorStateUnavailable is the state when it IS selected but
// the owner-gated real evaluator contract is missing (card 0428-152). These
// are the only two fail-closed states the consumer may show; a
// "healthy"/"verified" claim requires the Owner-approved real contract in
// internal/metrics and can never be produced here.
const (
	persesAlertEvaluatorStateNotConfigured = "not configured"
	persesAlertEvaluatorStateUnavailable   = "unavailable"
	persesAlertEvaluatorObservationName    = "management-ui/perses/alert-evaluator"
)

// persesAlertEvaluatorReachabilityVerified is emitted only when the live
// Prometheus rule-health verifier succeeds.
const persesAlertEvaluatorReachabilityVerified = "verified"

// persesAlertEvaluatorObservationWithVerifier maps the live evaluator probe to
// the one status observation shared by CLI and MCP. Missing deployment is not
// configured; a live healthy rule is verified; every other probe outcome is
// unavailable with the verifier's concrete fail-closed reason.
func persesAlertEvaluatorObservationWithVerifier(ctx context.Context, m application.Manifest, applied bool, notAppliedReason string, verify func(context.Context, application.Manifest) error) application.StatusCheck {
	if !applied {
		if notAppliedReason == "" {
			notAppliedReason = "this deployment was never applied"
		}
		return application.StatusCheck{
			Name: persesAlertEvaluatorObservationName, State: persesAlertEvaluatorStateNotConfigured,
			Detail: "Perses alert evaluator is " + persesAlertEvaluatorStateNotConfigured + ": " + notAppliedReason +
				"; no evaluator is wired on this surface",
		}
	}
	if err := verify(ctx, m); err != nil {
		state := persesAlertEvaluatorStateUnavailable
		if strings.Contains(err.Error(), "Perses is not selected") {
			state = persesAlertEvaluatorStateNotConfigured
		}
		return application.StatusCheck{
			Name: persesAlertEvaluatorObservationName, State: state,
			Detail: "Perses alert evaluator is " + state + " (live metrics verifier: " + err.Error() + ")",
		}
	}
	return application.StatusCheck{
		Name: persesAlertEvaluatorObservationName, State: persesAlertEvaluatorReachabilityVerified,
		Detail: "Perses alert evaluator is verified by the live Prometheus rule-health probe",
	}
}

func persesAlertEvaluatorObservation(ctx context.Context, m application.Manifest, applied bool, notAppliedReason string) application.StatusCheck {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return persesAlertEvaluatorObservationWithVerifier(checkCtx, m, applied, notAppliedReason, metricsprovider.VerifyPersesAlertEvaluatorHealth)
}

// deploymentApplied confirms both the materialized runtime files and the
// resolved deployment record's identity/runtime stamp before a live probe can
// run. Missing, unreadable, or mismatched state fails closed with a reason.
func deploymentApplied(resolved resolvedApplication) (bool, string) {
	files, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		return false, "the application runtime files are unavailable: " + err.Error()
	}
	if resolved.DeploymentIdentity.DeploymentID == "" {
		return false, "the resolved deployment identity is missing"
	}
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil {
		return false, "the resolved deployment record is unavailable: " + err.Error()
	}
	if record.Identity != resolved.DeploymentIdentity {
		return false, "the deployment record identity does not match the resolved deployment"
	}
	recordedRuntime := record.Applied.GeneratedState["runtime"]
	if recordedRuntime == "" || filepath.Clean(recordedRuntime) != filepath.Clean(files.Dir) {
		return false, "the runtime files are not stamped for the resolved deployment identity"
	}
	return true, ""
}

// reportPersesSurfaceStatus is the shared read seam behind 'baha app perses
// status' and the typed MCP operation: the public three-state contract from
// manifest and applied-runtime truth, never a health probe.
func reportPersesSurfaceStatus(ctx context.Context, store application.Store, args []string) (persesStatusResult, error) {
	resolved, err := resolveApplication(ctx, store, args, "perses status")
	if err != nil {
		return persesStatusResult{}, err
	}
	if err := authorizeApplicationOperation(ctx, persesLifecycleOperationID(persesLifecycleStatusAction), resolved); err != nil {
		return persesStatusResult{}, err
	}
	resolvedHostSignalStatus := capability.ResolveHostDashboardSignalStatus(
		resolved.Manifest.Services.ObservabilityManagementUI, resolved.Manifest.Services.Perses)
	applied, notAppliedReason := deploymentApplied(resolved)
	alertEvaluator := persesAlertEvaluatorObservation(ctx, resolved.Manifest, applied, notAppliedReason)
	return persesStatusResult{
		State:          application.PersesLifecycleStatus(resolved.Manifest),
		Enabled:        application.PersesLifecycleEnabled(resolved.Manifest),
		Version:        capability.PersesDefaultRelease.Version,
		BackendWitness: persesBackendSignalWitnessFromManifest(resolved.Manifest, resolvedHostSignalStatus, metricsprovider.PersesBackendSignalWitness{}),
		AlertEvaluator: &alertEvaluator,
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

// renderPersesBackendWitness prints the per-observability-signal three-state
// backend-witness the card mandates for `perses status` (configured /
// unreachable / not-configured), each line carrying its source-id. It never
// asserts a "healthy"/"verified" backend from a manifest flag; a real probe is
// card 0428-143. The machine `app status` host-containers JSON renders the
// identical witness through the shared persesBackendSignalWitnessFromManifest
// helper, so the two faces never drift.
func renderPersesBackendWitness(out io.Writer, witness []persesBackendSignalVerdict) {
	for _, verdict := range witness {
		fmt.Fprintf(out, "  %s [%s]: %s\n", verdict.Backend, verdict.SourceID, verdict.State)
	}
}
