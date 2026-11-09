package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// appPruneCommand is the ONLY registered command that removes a deployment
// record without an applied intent snapshot or a live source. It exists as the
// safe counterpart to the advice printed by `baha app list` for a MISSING
// deployment (0427-160 / card 0427-173): a source repository that is gone can
// no longer be resolved by the source-required lifecycle commands, so its
// deployment record is permanently uncollectable and the list keeps showing
// MISSING. prune removes JUST that dangling record from the registry.
//
// It never mutates runtime, containers, volumes, identities, secrets, provider
// bindings or any other deployment record. It is deliberately narrow:
//
//   - It refuses when MORE than one record matches the given name, naming each.
//     A plain NAME cannot disambiguate a name duplicate (0427-149), so a
//     matching name is refused until the operator picks a distinct environment
//     or deployment id.
//   - It refuses when the record's source repository is still present. A present
//     source means the deployment is recoverable and the lifecycle commands must
//     handle it; prune exists only for the unrecoverable case.
//   - It removes the record directory through the same exported seam other
//     registry mutations use, observing no owner/one-writer grant.
func appPruneCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "prune",
		Summary: "Remove a dangling deployment record whose source repository is gone",
		Usage:   "baha app prune <name>",
		Long:    "Removes a SINGLE dangling deployment record whose source repository is no longer available. The record can no longer be resolved by the source-required lifecycle commands (apply/update/plan/preflight/init) and cannot be destroyed or downed, so it stays permanently uncollectable and `baha app list` keeps showing MISSING. prune removes ONLY that registry record: it never touches runtime, containers, volumes, identities, secrets, provider bindings or any other deployment record. It refuses when the source repository is still present (the deployment is recoverable) and when the given name matches more than one record (naming each). Exactly one name is required.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			// Exactly ONE output form: the CLI selects human or structured JSON
			// from the parsed options. The operation returns a structured result
			// and never writes presentation text; this method renders exactly one
			// form so the buffer is either valid JSON or plain text, never both.
			filtered, format, err := parseReadOutputArgs(args, "app prune")
			if err != nil {
				return err
			}
			name, err := requirePruneName(filtered, "app prune")
			if err != nil {
				return err
			}
			if err := authorizeCurrentMCPContext(ctx, "app.prune", "", "", ""); err != nil {
				return err
			}
			if _, err := executeApplicationPrune(ctx, store, name, format, out, errOut); err != nil {
				return err
			}
			return nil
		},
	}
}

type applicationPruneResult struct {
	Target        string `json:"target"`
	Application   string `json:"application"`
	Environment   string `json:"environment"`
	DeploymentID  string `json:"deployment_id"`
	RecordRemoved bool   `json:"record_removed"`
	SourcePresent bool   `json:"source_present"`
}

func executeApplicationPrune(ctx context.Context, store application.Store, name string, format cliOutputFormat, out, errOut io.Writer) (applicationPruneResult, error) {
	result := applicationPruneResult{}

	// prune is a registry-identity command: it removes the dangling deployment
	// record whose source is gone. It resolves the record by exact name through
	// a dedicated resolver that reads only the deployment identity, its source
	// path and its applied-intent presence, WITHOUT loading the authored manifest
	// or resolving the applied backend selection (the source is gone, so that
	// indirection would fail). The one-name guard (0427-149) stays in force.
	resolved, err := resolvePruneCandidate(ctx, store, name)
	if err != nil {
		return result, err
	}
	return executeApplicationPruneResolved(ctx, store, resolved, name, format, out, errOut, nil)
}

// executeApplicationPruneResolved performs the guard chain and deletion on an
// ALREADY resolved record. The caller resolves once; this routine never
// resolves by name again. The CLI entry point resolves internally and delegates
// here with its own result; the MCP entry point resolves, authorizes that exact
// result, and then delegates here so authorization AND deletion operate on the
// same full deployment identity. An optional `authorizedIdentity` (only ever
// set by the MCP path) pins the identity that was authorized: the deletion
// removes exactly that record and the resolved record is validated against it
// before deletion so a same-name record replacement between authorization and
// mutation cannot redirect the deletion to a different deployment.
func executeApplicationPruneResolved(ctx context.Context, store application.Store, resolved resolvedApplication, name string, format cliOutputFormat, out, errOut io.Writer, authorizedIdentity *deployment.DeploymentIdentity) (applicationPruneResult, error) {
	result := applicationPruneResult{}
	if resolved.SourceAvailable {
		return result, &machine.Error{
			Code:        machine.ErrorSourceMissing,
			CauseCode:   "SOURCE_PRESENT",
			Message:     "source repository for " + resolved.ResourceLabel() + " is still present",
			Resource:    resolved.ResourceLabel(),
			Next:        "The deployment is recoverable: run it through the lifecycle command (e.g. 'baha up' or 'baha app destroy'), not prune.",
			Remediation: "operator_review",
		}
	}

	// A record with an applied intent snapshot is a real deployment that an
	// owner command must handle. prune exists only for the truly dangling case:
	// no applied intent snapshot and a source that is gone.
	if len(resolved.DeploymentRecord.Applied.Intent) > 0 {
		return result, &machine.Error{
			Code:        machine.ErrorUnsupported,
			CauseCode:   "RECORD_NOT_DANGLING",
			Message:     resolved.ResourceLabel() + " has an applied intent snapshot",
			Resource:    resolved.ResourceLabel(),
			Next:        "This is a real deployment: use 'baha app destroy' to retire it, not prune.",
			Remediation: "operator_review",
		}
	}

	if resolved.DeploymentRecord == nil {
		return result, &machine.Error{
			Code:      machine.ErrorNotFound,
			CauseCode: "deployment_record_not_found",
			Message:   "no deployment record could be resolved for " + resolved.ResourceLabel(),
			Resource:  resolved.ResourceLabel(),
			Next:      "Run 'baha app list --all-targets' to inspect registered deployments.",
		}
	}

	if err := deletionGuard(resolved); err != nil {
		return result, err
	}

	// Identity stability: the MCP path authorized a specific deployment and must
	// delete exactly that one. Pin the deletion to the authorized identity and
	// validate the freshly resolved identity against it before mutation, so a
	// same-name record replacement between authorization and deletion cannot
	// redirect the deletion to a different deployment. The CLI path leaves the
	// authorized identity nil and deletes the record it just resolved.
	if authorizedIdentity != nil {
		if !identitiesMatch(resolved, *authorizedIdentity) {
			return result, &machine.Error{
				Code:        machine.ErrorOwnershipAmbiguous,
				CauseCode:   "IDENTITY_DRIFT",
				Message:     "resolved identity no longer matches the authorized deployment for " + resolved.ResourceLabel(),
				Resource:    resolved.ResourceLabel(),
				Next:        "The deployment record changed between authorization and deletion; re-run to authorize the current record.",
				Remediation: "operator_review",
			}
		}
		if err := deployment.DeleteDeploymentRecord(*authorizedIdentity); err != nil {
			return result, pruneDeletionError(resolved, err)
		}
	} else {
		if err := deployment.DeleteDeploymentRecord(resolved.DeploymentIdentity); err != nil {
			return result, pruneDeletionError(resolved, err)
		}
	}

	result.Target = resolved.Target.Name
	result.Application = resolved.DeploymentRecord.Identity.Application
	result.Environment = resolved.DeploymentRecord.Identity.Environment
	result.DeploymentID = resolved.DeploymentRecord.Identity.DeploymentID
	result.RecordRemoved = true
	result.SourcePresent = false

	// Emit exactly one output form selected from the parsed options: JSON when
	// requested, plain human text otherwise. The operation writes no presentation
	// text itself, so a JSON request never carries a human trailer and a human
	// request never carries a JSON trailer.
	if format == outputJSON {
		return result, writeJSON(out, result)
	}
	fmt.Fprintf(out, "Removed dangling deployment record %s\n", resolved.ResourceLabel())
	fmt.Fprintf(out, "  target:       %s\n", resolved.Target.Name)
	fmt.Fprintf(out, "  application:  %s\n", resolved.DeploymentRecord.Identity.Application)
	fmt.Fprintf(out, "  environment:  %s\n", resolved.DeploymentRecord.Identity.Environment)
	fmt.Fprintf(out, "  deployment:   %s\n", resolved.DeploymentRecord.Identity.DeploymentID)
	fmt.Fprintf(out, "  source:       MISSING (source repository gone)\n")
	fmt.Fprintf(out, "No runtime, containers, volumes, identities, secrets or provider bindings were touched. No other deployment record was changed.\n")
	return result, nil
}

// pruneDeletionError translates a deletion failure into a machine error for
// structured clients. A drift detected by the on-disk identity validation
// (the record that now lives on disk is not the one that was authorized)
// surfaces with the IDENTITY_DRIFT cause and fails closed; any other removal
// failure wraps the underlying error.
func pruneDeletionError(resolved resolvedApplication, err error) *machine.Error {
	resource := resolved.ResourceLabel()
	var drifted *machine.Error
	if errors.As(err, &drifted) {
		return drifted
	}
	return &machine.Error{
		Code:        machine.ErrorInternal,
		CauseCode:   "deletion_failed",
		Message:     fmt.Sprintf("remove dangling deployment record %s: %v", resource, err),
		Resource:    resource,
		Next:        "Re-run to retry; if the record changed between authorization and deletion the cause is IDENTITY_DRIFT.",
		Remediation: "operator_review",
	}
}

// identitiesMatch reports whether the resolved record's full deployment identity
// equals the identity that was authorized. prune authorizes one deployment and
// must delete exactly that one, so any difference — a same-name record replaced
// on the target between authorization and mutation — is a drift that fails
// closed rather than redirecting the deletion.
func identitiesMatch(resolved resolvedApplication, authorized deployment.DeploymentIdentity) bool {
	if resolved.DeploymentRecord == nil {
		return false
	}
	a := resolved.DeploymentRecord.Identity
	return a.Target == authorized.Target &&
		a.DeploymentID == authorized.DeploymentID &&
		a.ApplicationID == authorized.ApplicationID &&
		a.Application == authorized.Application &&
		a.Environment == authorized.Environment
}

// deletionGuard is the narrow fail-closed gate that protects every foreign or
// recoverable deployment. It runs AFTER resolveApplication has pinned exactly
// one record by name, so it only has to reject a present source or an
// applied-intent snapshot. It mutates nothing and reads nothing it does not
// already hold through the resolved record.
func deletionGuard(resolved resolvedApplication) error {
	// A present source means the deployment is recoverable and the owner must
	// handle it through the lifecycle command. prune exists only for the
	// unrecoverable case.
	if resolved.SourceAvailable {
		return &machine.Error{
			Code:        machine.ErrorSourceMissing,
			CauseCode:   "SOURCE_PRESENT",
			Message:     "source repository for " + resolved.ResourceLabel() + " is still present",
			Resource:    resolved.ResourceLabel(),
			Next:        "The deployment is recoverable: run it through the lifecycle command (e.g. 'baha up' or 'baha app destroy'), not prune.",
			Remediation: "operator_review",
		}
	}
	// An applied intent snapshot is a real deployment that an owner command must
	// handle; prune exists only for the truly dangling case.
	if len(resolved.DeploymentRecord.Applied.Intent) > 0 {
		return &machine.Error{
			Code:        machine.ErrorUnsupported,
			CauseCode:   "RECORD_NOT_DANGLING",
			Message:     resolved.ResourceLabel() + " has an applied intent snapshot",
			Resource:    resolved.ResourceLabel(),
			Next:        "This is a real deployment: use 'baha app destroy' to retire it, not prune.",
			Remediation: "operator_review",
		}
	}
	return nil
}

// resolvePruneCandidate pins exactly one deployment record by exact NAME,
// returning a resolvedApplication that carries only the identity, source path,
// source availability and applied-intent presence prune needs. Name resolution
// is delegated to a single resolver shared with the CLI and MCP entry points so
// both refuse zero and multiple names identically. It deliberately skips the
// authored-manifest load and the applied-backend-selection resolution that
// every other app command performs: the dangling record's source is gone, so
// that indirection would fail before prune could remove it.
func resolvePruneCandidate(ctx context.Context, store application.Store, name string) (resolvedApplication, error) {
	switch len(name) {
	case 0:
		return resolvedApplication{}, usageError("app prune requires exactly one deployment name", "Use 'baha app prune <name>' to remove one dangling record. A bare 'baha app prune' with no name selects nothing by design.")
	default:
		return resolvePruneByName(ctx, name)
	}
}

// requirePruneName enforces the one-name contract at the CLI entry: exactly one
// positional argument, no stray flags or extra names. It returns the single
// name the rest of the command operates on.
func requirePruneName(args []string, command string) (string, error) {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return "", usageError("unknown option "+arg, "Use 'baha app prune <name> [--json]'; prune requires exactly one deployment name.")
		}
	}
	switch len(args) {
	case 0:
		return "", usageError("missing deployment name", "Use 'baha app prune <name>' to remove one dangling record. A bare 'baha app prune' with no name selects nothing by design.")
	case 1:
		return args[0], nil
	default:
		return "", usageError(fmt.Sprintf("app prune accepts exactly one name, got %d", len(args)), "Use 'baha app prune <name>' to remove one dangling record.")
	}
}

// resolvePruneByName pins exactly one deployment record by exact NAME, naming
// every match when a name is ambiguous (0427-149 name-duplicate guard). It
// deliberately skips the authored-manifest load and the applied-backend-selection
// resolution that every other app command performs: the dangling record's
// source is gone, so that indirection would fail before prune could remove it.
//
// It preserves any target the caller selected on the path to this resolver
// (the MCP handler pins `input.Target`, the CLI entry pins the invocation
// target). Only when the caller carried no explicit target does it fall back to
// the ambient BASEHARBOR_TARGET/default resolution. Silently clearing the
// override here would let the MCP prune resolve the ambient target instead of
// the one the caller authorized, so authorization and the deletion could act on
// different deployments.
func resolvePruneByName(ctx context.Context, name string) (resolvedApplication, error) {
	if targetOverrideFromContext(ctx) == "" {
		ctx = withTargetOverride(ctx, "")
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return resolvedApplication{}, err
	}
	targetRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		return resolvedApplication{}, err
	}
	records, listErr := deployment.ListDeployments(target.Name)
	if listErr != nil {
		return resolvedApplication{}, listErr
	}

	// Exact NAME match. The name is the single unambiguous selector prune acts
	// on: it must match exactly one record, or be refused naming each.
	var matches []deployment.DeploymentRecord
	for i := range records {
		if records[i].Identity.Application != name {
			continue
		}
		matches = append(matches, records[i])
	}
	if len(matches) == 0 {
		return resolvedApplication{}, &machine.Error{
			Code:      machine.ErrorNotFound,
			CauseCode: "application_deployment_not_found",
			Message:   fmt.Sprintf("application %q has no registered deployment on target %q", name, target.Name),
			Resource:  target.Name + "/" + name,
			Next:      "Run 'baha app list' to inspect registered applications.",
		}
	}
	if len(matches) > 1 {
		byEnv := make(map[string][]string)
		environments := make([]string, 0, len(matches))
		for i := range matches {
			environments = append(environments, matches[i].Identity.Environment)
			byEnv[matches[i].Identity.Environment] = append(byEnv[matches[i].Identity.Environment], matches[i].Identity.DeploymentID)
		}
		perEnvironment := make([]string, 0, len(byEnv))
		for env, ids := range byEnv {
			perEnvironment = append(perEnvironment, env+": "+strings.Join(ids, ", "))
		}
		return resolvedApplication{}, &machine.Error{
			Code:        machine.ErrorOwnershipAmbiguous,
			CauseCode:   "application_name_ambiguous",
			Message:     fmt.Sprintf("application %q has multiple registered deployments on target %q: %v", name, target.Name, strings.Join(perEnvironment, "; ")),
			Resource:    target.Name + "/" + name,
			Next:        "baha app prune refuses an ambiguous name until it selects exactly one deployment; inspect 'baha app list --all-targets' to see the matches.",
			Remediation: "operator_review",
		}
	}
	return prunedResolved(target, targetRoot, matches[0]), nil
}

// prunedResolved builds the minimal resolvedApplication prune needs: identity,
// target roots, source path, source availability and the applied-intent
// presence. It loads no manifest and resolves no backend selection.
func prunedResolved(target deployment.ResolvedTarget, targetRoot string, record deployment.DeploymentRecord) resolvedApplication {
	deploymentRoot, _ := deployment.DeploymentRoot(record.Identity)
	return resolvedApplication{
		Target:              target,
		DeploymentIdentity:  record.Identity,
		DeploymentRecord:    &record,
		TargetStateRoot:     targetRoot,
		DeploymentStateRoot: deploymentRoot,
		RepositoryRoot:      record.Source.Repository,
		SourceAvailable:     deployment.SourceAvailable(record.Source),
	}
}

// registerMCPAppPruneTool exposes the dangling-record removal through the typed
// MCP tool boundary. It is a destructive, approval-required operation, so the
// handler authorizes `app.prune` on the resolved identity before removing the
// record. The machine result mirrors the CLI result shape so structured
// clients receive the same confirmation.
func registerMCPAppPruneTool(server *mcp.Server, store application.Store) {
	mcp.AddTool(server, machineMCPTool("app.prune", "Remove a single dangling deployment record whose source repository is gone (refuses when source is present or multiple records match).", true),
		func(ctx context.Context, _ *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
			ctx = withTargetOverride(ctx, input.Target)
			resolved, err := resolvePruneCandidate(ctx, store, strings.TrimSpace(input.Name))
			if err != nil {
				return machineMCPFailure(err)
			}
			// Authorize the exact resolved identity, then carry that SAME identity
			// into the mutation. executeApplicationPruneResolved validates the
			// resolved identity against this pinned value before deleting, so the
			// authorization and the deletion operate on the same deployment and a
			// same-name record replacement between the two cannot redirect the
			// deletion.
			authorizedIdentity := resolved.DeploymentIdentity
			if err := authorizeResolvedMCPOperation(ctx, "app.prune", resolved, ""); err != nil {
				return machineMCPFailure(err)
			}
			outBuf, errOutBuf := &bytes.Buffer{}, &bytes.Buffer{}
			_, rErr := executeApplicationPruneResolved(ctx, store, resolved, strings.TrimSpace(input.Name), outputJSON, outBuf, errOutBuf, &authorizedIdentity)
			if rErr != nil {
				return machineMCPFailure(rErr)
			}
			return nil, resultFromPruneOutput(outBuf.Bytes()), nil
		})
}

// resultFromPruneOutput parses the JSON body the prune command writes so the
// MCP handler returns the same machine result the CLI emits.
func resultFromPruneOutput(data []byte) applicationPruneResult {
	var result applicationPruneResult
	if err := json.Unmarshal(data, &result); err != nil {
		return result
	}
	return result
}
