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
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
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
//   - status reports the public three-state contract and probes each selected
//     observability backend through its existing authorized query path.
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
	return persesStatusCommandWithProbes(store, persesBackendSignalProbes())
}

func persesStatusCommandWithProbes(store application.Store, probes persesBackendSignalProbeSet) *cli.Command {
	return &cli.Command{
		Name:    "status",
		Summary: "Report the Perses three-state contract",
		Usage:   "baha app perses status [NAME] [--json]",
		Long:    "Reports the public Perses lifecycle state plus a fail-closed per-backend source witness. The lifecycle state remains not_configured or configured_unverified; backend source answers are observed independently through authorized Prometheus, Loki and Tempo query paths.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "perses status")
			if err != nil {
				return err
			}
			result, err := reportPersesSurfaceStatusWithProbes(ctx, store, filtered, probes)
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			fmt.Fprintf(out, "Perses state: %s\n", result.State)
			renderPersesBackendWitness(out, result.BackendWitness)
			return nil
		},
	}
}

type persesBackendSignalProbeSet struct {
	prometheus func(context.Context, application.Manifest) error
	loki       func(context.Context, application.Manifest) error
	tempo      func(context.Context, application.Manifest) error
}

func persesBackendSignalProbes() persesBackendSignalProbeSet {
	return persesBackendSignalProbeSet{
		prometheus: func(ctx context.Context, manifest application.Manifest) error {
			status := metricsprovider.PersesBackendAnswersStatusNow(ctx, manifest)
			if status == nil || *status != capability.PersesStatusVerified {
				return errors.New("Prometheus provider-metric query did not produce a live answer")
			}
			return nil
		},
		loki:  logsprovider.VerifyPersesLokiDatasourceHealth,
		tempo: metricsprovider.VerifyPersesTempoDatasourceHealth,
	}
}

func persesBackendSignalProbesWithPrometheusStatus(status *capability.PersesStatus) persesBackendSignalProbeSet {
	return persesBackendSignalProbeSet{
		prometheus: func(context.Context, application.Manifest) error {
			if status == nil || *status != capability.PersesStatusVerified {
				return errors.New("Prometheus provider-metric query did not produce a live answer")
			}
			return nil
		},
		loki:  logsprovider.VerifyPersesLokiDatasourceHealth,
		tempo: metricsprovider.VerifyPersesTempoDatasourceHealth,
	}
}

func observePersesBackendSignalWitness(ctx context.Context, manifest application.Manifest, probes persesBackendSignalProbeSet) metricsprovider.PersesBackendSignalWitness {
	var witness metricsprovider.PersesBackendSignalWitness
	if !manifest.Services.ObservabilityManagementUI || !manifest.Services.Perses {
		return witness
	}
	if application.HasMetricsSources(manifest) {
		witness.PrometheusProven = probes.prometheus != nil && probes.prometheus(ctx, manifest) == nil
	}
	if application.HasLogsCollection(manifest) {
		witness.LokiProven = probes.loki != nil && probes.loki(ctx, manifest) == nil
	}
	if application.HasOTLPTelemetry(manifest) {
		witness.TempoProven = probes.tempo != nil && probes.tempo(ctx, manifest) == nil
	}
	return witness
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
	// BackendWitness names each declared backend and its independent source
	// observation. A configured verdict requires that backend's authorized
	// query to return a match; it does not claim general service health.
	BackendWitness []persesBackendSignalVerdict `json:"backend_witness,omitempty"`
	// AlertEvaluator is the single honest consumer of the Perses alert-
	// evaluator verifier contract on this command face (card 0428-152): the
	// state is derived from exactly one BuildPersesAlertEvaluator call and is
	// one of "not configured", "unavailable (owner-gated evaluator contract)"
	// or the verifier's reachability shown verbatim ("configured_unverified"
	// in this Core). It never reports healthy/verified without the
	// Owner-approved real contract. See persesAlertEvaluatorObservation.
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
	persesAlertEvaluatorStateUnavailable   = "unavailable (owner-gated evaluator contract)"
	persesAlertEvaluatorObservationName    = "management-ui/perses/alert-evaluator"
)

// persesAlertEvaluatorReachabilityVerified is the verifier's own marker for a
// contract that has been actually verified by internal/metrics. No producer in
// this Core sets it (persesAlertRealEvaluatorContractPresent keeps the
// verifier fail-closed), and as long as no verifier result carries it, the
// consumer can never report "healthy"/"verified".
const persesAlertEvaluatorReachabilityVerified = "verified"

// persesAlertEvaluatorObservation is the single honest consumer of the Perses
// alert-evaluator verifier contract in internal/metrics
// (perses_prometheus_alerts.go), shared by every status face ('baha app
// perses status' and 'baha app status'). The decision comes from exactly one
// verifier call, BuildPersesAlertEvaluator, never from a second parallel
// logic in the CLI:
//
//   - The verifier fails closed with "Perses is not selected" => "not
//     configured" (the manifest does not select it).
//   - The verifier fails closed for any other reason (no metric inputs,
//     missing evaluator contract, invalid pinned release) => "unavailable
//     (owner-gated evaluator contract)" (selected, contract missing).
//   - The verifier succeeds => its reachability string is shown verbatim
//     ("configured_unverified" in this Core). A hypothetical "verified"
//     reachability maps to the unavailable state instead of being echoed, so
//     "healthy"/"verified" can never fall out at the consumer (fail-closed
//     here, not only in the verifier).
//
// The state string is never "ready"/"verified", so the observation never
// folds into readiness, and no new state holder or manifest field exists:
// the decision derives from the existing manifest bindings the verifier
// itself reads.
func persesAlertEvaluatorObservation(m application.Manifest) application.StatusCheck {
	result, err := metricsprovider.BuildPersesAlertEvaluator(m)
	if err != nil {
		state := persesAlertEvaluatorStateUnavailable
		if strings.Contains(err.Error(), "Perses is not selected") {
			state = persesAlertEvaluatorStateNotConfigured
		}
		return application.StatusCheck{
			Name:   persesAlertEvaluatorObservationName,
			State:  state,
			Detail: "Perses alert evaluator is " + state + " (fail-closed reason from the metrics verifier: " + err.Error() + "); this Core never reports the evaluator healthy or verified",
		}
	}
	state := result.Reachability
	if state == persesAlertEvaluatorReachabilityVerified {
		state = persesAlertEvaluatorStateUnavailable
	}
	return application.StatusCheck{
		Name:   persesAlertEvaluatorObservationName,
		State:  state,
		Detail: fmt.Sprintf("Perses alert evaluator: kind %s, owner %s, verifier reachability %q shown verbatim; no live rule evaluation is claimed", result.Evaluator.Kind, result.Evaluator.Owner, result.Reachability),
	}
}

// persesAlertEvaluatorObservationWithApplication is the perses status face of
// the same single consumer, plus the one truth the manifest verifier
// cannot see: whether any deployment was ever applied. The state machine is
// three distinct states:
//
//   - the runtime was never applied (no deployment wires an evaluator) =>
//     "not configured", whatever the manifest selects;
//   - applied but the Owner-gated real contract is missing => "unavailable
//     (owner-gated evaluator contract)";
//   - applied with the wiring the verifier can show => its reachability
//     verbatim ("configured_unverified" in this Core).
//
// The decision still comes from exactly one BuildPersesAlertEvaluator call;
// the applied flag only suppresses any "configured" claim for wiring that does
// not exist yet (fail-closed), and adds no state holder or manifest field.
//
// When the runtime was never applied, no evaluator is wired: the state is
// "not configured" and the detail says so plainly. The unselected manifest's
// verifier reason ("Perses is not selected") is pinned by the manifest-only
// contract cases, which call persesAlertEvaluatorObservation directly; the
// not-applied face makes no manifest or verifier claim at all.
func persesAlertEvaluatorObservationWithApplication(m application.Manifest, applied bool) application.StatusCheck {
	if applied {
		return persesAlertEvaluatorObservation(m)
	}
	// Even on the not-applied face, the verifier's own fail-closed reason is
	// honest when the manifest does not select Perses at all. Preserve it
	// verbatim; the "never applied" wording is for the selected case only.
	if _, err := metricsprovider.BuildPersesAlertEvaluator(m); err != nil && strings.Contains(err.Error(), "Perses is not selected") {
		return application.StatusCheck{
			Name:  persesAlertEvaluatorObservationName,
			State: persesAlertEvaluatorStateNotConfigured,
			Detail: "Perses alert evaluator is " + persesAlertEvaluatorStateNotConfigured +
				" (fail-closed reason from the metrics verifier: " + err.Error() + "); no deployment was applied, so this surface is not configured",
		}
	}
	return application.StatusCheck{
		Name:  persesAlertEvaluatorObservationName,
		State: persesAlertEvaluatorStateNotConfigured,
		Detail: "Perses alert evaluator is " + persesAlertEvaluatorStateNotConfigured +
			": this deployment was never applied, so no evaluator is wired on this surface; no manifest or verifier claim is made",
	}
}

// deploymentApplied reports whether the resolved application's runtime has
// ever been applied: the production collection path treats the absence of
// materialized runtime files (application.ErrRuntimeNotApplied from
// ExistingRuntimeFiles) as the not-applied face, so this seam asks exactly
// the same question, on the same store the runtime writer uses. A missing,
// unreadable or absent runtime tree is honest "not applied" (fail-closed):
// only materialized, applied runtime may claim a wired evaluator.
func deploymentApplied(resolved resolvedApplication) bool {
	_, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	return err == nil
}

// reportPersesSurfaceStatus is the shared read seam behind 'baha app perses
// status' and the typed MCP operation. Authorization succeeds before the
// backend-specific read probes run; every probe fails closed independently.
func reportPersesSurfaceStatus(ctx context.Context, store application.Store, args []string) (persesStatusResult, error) {
	return reportPersesSurfaceStatusWithProbes(ctx, store, args, persesBackendSignalProbes())
}

func reportPersesSurfaceStatusWithProbes(ctx context.Context, store application.Store, args []string, probes persesBackendSignalProbeSet) (persesStatusResult, error) {
	resolved, err := resolveApplication(ctx, store, args, "perses status")
	if err != nil {
		return persesStatusResult{}, err
	}
	if err := authorizeApplicationOperation(ctx, persesLifecycleOperationID(persesLifecycleStatusAction), resolved); err != nil {
		return persesStatusResult{}, err
	}
	resolvedHostSignalStatus := capability.ResolveHostDashboardSignalStatus(
		resolved.Manifest.Services.ObservabilityManagementUI, resolved.Manifest.Services.Perses)
	alertEvaluator := persesAlertEvaluatorObservationWithApplication(resolved.Manifest, deploymentApplied(resolved))
	backendWitness := observePersesBackendSignalWitness(ctx, resolved.Manifest, probes)
	return persesStatusResult{
		State:          application.PersesLifecycleStatus(resolved.Manifest),
		Enabled:        application.PersesLifecycleEnabled(resolved.Manifest),
		Version:        capability.PersesDefaultRelease.Version,
		BackendWitness: persesBackendSignalWitnessFromManifest(resolved.Manifest, resolvedHostSignalStatus, backendWitness),
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
// unreachable / not-configured), each line carrying its source-id. A configured
// verdict comes from that backend's own authorized query; it does not claim
// general service health. The machine `app status` JSON renders the
// identical witness through the shared persesBackendSignalWitnessFromManifest
// helper, so the two faces never drift.
func renderPersesBackendWitness(out io.Writer, witness []persesBackendSignalVerdict) {
	for _, verdict := range witness {
		fmt.Fprintf(out, "  %s [%s]: %s\n", verdict.Backend, verdict.SourceID, verdict.State)
	}
}
