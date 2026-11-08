package main

import (
	"bytes"
	"context"
	"encoding/json"
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
//     A plain NAME cannot disambiguate a name duplicate (0427-149), so the
//     operator must pin one with --uuid or a distinct -e/--environment.
//   - It refuses when the record's source repository is still present. A present
//     source means the deployment is recoverable and the lifecycle commands must
//     handle it; prune exists only for the unrecoverable case.
//   - It removes the record directory through the same exported seam other
//     registry mutations use, observing no owner/one-writer grant.
func appPruneCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "prune",
		Summary: "Remove a dangling deployment record whose source repository is gone",
		Usage:   "baha app prune [NAME] [--uuid <deployment-id>]",
		Long:    "Removes a SINGLE dangling deployment record whose source repository is no longer available. The record can no longer be resolved by the source-required lifecycle commands (apply/update/plan/preflight/init) and cannot be destroyed or downed, so it stays permanently uncollectable and `baha app list` keeps showing MISSING. prune removes ONLY that registry record: it never touches runtime, containers, volumes, identities, secrets, provider bindings or any other deployment record. It refuses when the source repository is still present (the deployment is recoverable) and when more than one record matches the given name (naming each); use --uuid or a distinct -e/--environment to pin one.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if _, _, err := parseReadOutputArgs(args, "app prune"); err != nil {
				return err
			}
			for _, arg := range args {
				if strings.HasPrefix(arg, "-") {
					return &cli.UsageError{Message: "unknown option " + arg, Hint: "Use baha app prune [NAME] [--uuid <deployment-id>]; prune removes a single dangling record and requires an exact name (or --uuid)."}
				}
			}
			if err := authorizeCurrentMCPContext(ctx, "app.prune", "", "", ""); err != nil {
				return err
			}
			result, err := executeApplicationPrune(ctx, store, args, out, errOut)
			if err != nil {
				return err
			}
			return writeJSON(out, result)
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

func executeApplicationPrune(ctx context.Context, store application.Store, args []string, out, errOut io.Writer) (applicationPruneResult, error) {
	result := applicationPruneResult{}

	// prune is a registry-identity command: it removes the dangling deployment
	// record whose source is gone. It resolves the record by exact name (or
	// --uuid) through a dedicated resolver that reads only the deployment
	// identity, its source path and its applied-intent presence, WITHOUT loading
	// the authored manifest or resolving the applied backend selection (the
	// source is gone, so that indirection would fail). --uuid and -e are honored
	// here so the name-duplicate guard (0427-149) stays in force.
	resolved, err := resolvePruneCandidate(ctx, store, args)
	if err != nil {
		return result, err
	}
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

	if err := deployment.DeleteDeploymentRecord(resolved.DeploymentIdentity); err != nil {
		return result, fmt.Errorf("remove dangling deployment record %s: %w", resolved.ResourceLabel(), err)
	}

	result.Target = resolved.Target.Name
	result.Application = resolved.DeploymentRecord.Identity.Application
	result.Environment = resolved.DeploymentRecord.Identity.Environment
	result.DeploymentID = resolved.DeploymentRecord.Identity.DeploymentID
	result.RecordRemoved = true
	result.SourcePresent = false
	fmt.Fprintf(out, "Removed dangling deployment record %s\n", resolved.ResourceLabel())
	fmt.Fprintf(out, "  target:       %s\n", resolved.Target.Name)
	fmt.Fprintf(out, "  application:  %s\n", resolved.DeploymentRecord.Identity.Application)
	fmt.Fprintf(out, "  environment:  %s\n", resolved.DeploymentRecord.Identity.Environment)
	fmt.Fprintf(out, "  deployment:   %s\n", resolved.DeploymentRecord.Identity.DeploymentID)
	fmt.Fprintf(out, "  source:       MISSING (source repository gone)\n")
	fmt.Fprintf(out, "No runtime, containers, volumes, identities, secrets or provider bindings were touched. No other deployment record was changed.\n")
	return result, nil
}

// deletionGuard is the narrow fail-closed gate that protects every foreign or
// recoverable deployment. It runs AFTER resolveApplication has pinned exactly
// one record (name match or --uuid), so it only has to reject a present source
// or an applied-intent snapshot. It mutates nothing and reads nothing it does
// not already hold through the resolved record.
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

// resolvePruneCandidate pins exactly one deployment record by exact NAME (with
// optional -e/--environment) or --uuid, returning a resolvedApplication that
// carries only the identity, source path, source availability and applied-intent
// presence prune needs. It deliberately skips the authored-manifest load and the
// applied-backend-selection resolution that every other app command performs:
// the dangling record's source is gone, so that indirection would fail before
// prune could remove it. The name-duplicate guard lives here, mirroring
// resolveRegisteredApplication's name filter.
func resolvePruneCandidate(ctx context.Context, store application.Store, args []string) (resolvedApplication, error) {
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

	// --uuid routes directly at the registry seam, bypassing the name filter
	// entirely: it is the unambiguous identity for a name-colliding deployment
	// that no -e value can disambiguate (0427-149).
	deploymentUUID, uuidErr := extractApplicationUUID(args)
	if uuidErr != nil {
		return resolvedApplication{}, uuidErr
	}
	if strings.TrimSpace(deploymentUUID) != "" {
		for i := range records {
			if records[i].Identity.DeploymentID == deploymentUUID {
				record := records[i]
				return prunedResolved(target, targetRoot, record), nil
			}
		}
		return resolvedApplication{}, &machine.Error{
			Code:      machine.ErrorNotFound,
			CauseCode: "deployment_uuid_not_found",
			Message:   fmt.Sprintf("deployment %q has no registered deployment on target %q", deploymentUUID, target.Name),
			Resource:  target.Name + "/" + deploymentUUID,
			Next:      "Run 'baha app list --all-targets' to inspect registered deployments and their deployment ids.",
		}
	}

	// Exact NAME match (optionally filtered by environment). A name that matches
	// more than one record is refused, naming each, because prune must remove a
	// single record and cannot safely disambiguate a name duplicate.
	environment := ""
	if len(args) > 0 {
		environment = args[0]
	}
	var matches []deployment.DeploymentRecord
	for i := range records {
		if environment != "" && records[i].Identity.Application != environment {
			continue
		}
		matches = append(matches, records[i])
	}
	if len(matches) == 0 {
		return resolvedApplication{}, &machine.Error{
			Code:      machine.ErrorNotFound,
			CauseCode: "application_deployment_not_found",
			Message:   fmt.Sprintf("application %q has no registered deployment on target %q", environment, target.Name),
			Resource:  target.Name + "/" + environment,
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
			Message:     fmt.Sprintf("application %q has multiple registered deployments on target %q: %v", environment, target.Name, strings.Join(perEnvironment, "; ")),
			Resource:    target.Name + "/" + environment,
			Next:        "Select one deployment by uuid: baha app prune --uuid <deployment-id>, or by a distinct environment with -e/--environment.",
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
			args := machineApplicationArgs(input.Name, input.Environment)
			resolved, err := resolvePruneCandidate(ctx, store, args)
			if err != nil {
				return machineMCPFailure(err)
			}
			if err := authorizeResolvedMCPOperation(ctx, "app.prune", resolved, ""); err != nil {
				return machineMCPFailure(err)
			}
			outBuf, errOutBuf := &bytes.Buffer{}, &bytes.Buffer{}
			_, rErr := executeApplicationPrune(ctx, store, args, outBuf, errOutBuf)
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
