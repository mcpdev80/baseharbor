package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// refuseRegistryApplicationIDConflict checks the Target's deployment
// registry for the application_id poison and reports it with the canonical
// inventory classification (a structured machine error, so the CLI, MCP and
// JSON surfaces render the same repair text). A healthy registry is a
// no-op read; the tolerant display listing makes detection survive the very
// damage it looks for (0427-175).
func refuseRegistryApplicationIDConflict(targetName string) error {
	records, _, err := deployment.ListDeploymentsForDisplay(targetName)
	if err != nil {
		return invalidProviderConsumerInventory(err)
	}
	if err := deployment.RefuseApplicationIDConflictOnDisplay(targetName, records); err != nil {
		return invalidProviderConsumerInventory(err)
	}
	return nil
}

// withoutUUIDSelector returns args with any --uuid option pair removed, so the
// positional-NAME limit counts only real NAME arguments (0427-175: MCP and CLI
// callers pass NAME plus an explicit uuid selector together).
func withoutUUIDSelector(args []string) []string {
	var rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--uuid":
			i++
		case strings.HasPrefix(arg, "--uuid="):
		default:
			rest = append(rest, arg)
		}
	}
	return rest
}

// withoutEnvironmentSelector returns args with any -e/--environment option
// pair removed, mirroring extractApplicationEnvironment: the pair is an
// option, never a positional NAME, so it must not count toward the one-NAME
// limit on the resolution paths that do not run the full command-line parse.
func withoutEnvironmentSelector(args []string) []string {
	var rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-e" || arg == "--environment":
			i++
		case strings.HasPrefix(arg, "--environment="):
		default:
			rest = append(rest, arg)
		}
	}
	return rest
}

// resolveRecordByIdentity reconstructs a resolved application from a single
// authoritative deployment record, provenance-verified.
//
// The optional selection argument names the deployment the operator addressed
// explicitly (the uuid-addressed repair); what that changes is at
// recordSourceApplication and appliedBackendSelection. The repair deletes a
// directory, so the identity resolved here must be the identity selected, and
// it is rechecked before returning (0427-175). Nothing here writes.
func resolveRecordByIdentity(ctx context.Context, target deployment.ResolvedTarget, targetRoot string, record deployment.DeploymentRecord, command string, selection ...string) (resolvedApplication, error) {
	selected := ""
	if len(selection) > 0 {
		selected = strings.TrimSpace(selection[0])
	}
	resolved, err := resolveRecordByIdentityProjecting(ctx, target, targetRoot, record, command, selected)
	if err != nil {
		return resolvedApplication{}, err
	}
	if selected != "" && resolved.DeploymentIdentity.DeploymentID != selected {
		// One unbroken chain, checked once at its end: the resolution above
		// keeps the record's own identity through source hydration and the
		// backend projection, so if the resolved deployment is not the one the
		// operator selected, some seam routed to the sibling. The repair
		// deletes a directory, so it must stop before any mutation (0427-175).
		return resolvedApplication{}, fmt.Errorf("deployment %s did not route the resolution: %s was resolved", selected, resolved.DeploymentIdentity.DeploymentID)
	}
	return resolved, nil
}

// resolveRecordByIdentityProjecting hydrates a located record from its source
// or applied intent, keeping the record's identity unchanged. A non-empty
// selection makes the record (not the registry) authoritative for identity;
// see recordSourceApplication and appliedBackendSelection. It finds no record,
// allocates no state and writes nothing (0427-175).
func resolveRecordByIdentityProjecting(ctx context.Context, target deployment.ResolvedTarget, targetRoot string, record deployment.DeploymentRecord, command, selectedDeploymentID string) (resolvedApplication, error) {
	deploymentRoot, err := deployment.DeploymentRoot(record.Identity)
	if err != nil {
		return resolvedApplication{}, err
	}

	resolved := resolvedApplication{
		Target:              target,
		DeploymentIdentity:  record.Identity,
		DeploymentRecord:    &record,
		TargetStateRoot:     targetRoot,
		DeploymentStateRoot: deploymentRoot,
		Store:               application.Store{Root: filepath.Join(deploymentRoot, "state"), Namespace: target.Name},
		RepositoryRoot:      record.Source.Repository,
		ManifestPath:        record.Source.Manifest,
		SourceAvailable:     deployment.SourceAvailable(record.Source),
	}

	if resolved.SourceAvailable {
		selection, sourceErr := application.ResolveRepositoryEnvironment(record.Source.Repository, record.Identity.Environment)
		switch {
		case sourceErr != nil:
			resolved.SourceAvailable = false
		case selection.Manifest.Name != record.Identity.Application || selection.Manifest.Environment != record.Identity.Environment:
			return resolvedApplication{}, fmt.Errorf("registered source does not match authoritative deployment name/environment")
		default:
			return recordSourceApplication(ctx, target, resolved, record, selection, command, selectedDeploymentID)
		}
	}

	if commandRequiresLiveSource(command) {
		return resolvedApplication{}, &machine.Error{
			Code:      machine.ErrorSourceMissing,
			CauseCode: "SOURCE_MISSING",
			Message:   fmt.Sprintf("source repository for %s/%s/%s is unavailable", record.Identity.Target, record.Identity.Application, record.Identity.Environment),
			Resource:  record.Identity.Target + "/" + record.Identity.Application + "/" + record.Identity.Environment,
			Next:      "Restore the registered repository path or run the operation from an available repository source.",
		}
	}
	if len(record.Applied.Intent) == 0 {
		return resolvedApplication{}, fmt.Errorf("deployment %s/%s/%s has no applied intent snapshot", record.Identity.Target, record.Identity.Application, record.Identity.Environment)
	}
	if err := json.Unmarshal(record.Applied.Intent, &resolved.Manifest); err != nil {
		return resolvedApplication{}, fmt.Errorf("decode applied intent for %s/%s/%s: %w", record.Identity.Target, record.Identity.Application, record.Identity.Environment, err)
	}
	if err := resolved.Manifest.Validate(); err != nil {
		return resolvedApplication{}, fmt.Errorf("validate applied intent for %s/%s/%s: %w", record.Identity.Target, record.Identity.Application, record.Identity.Environment, err)
	}
	return resolved, nil
}
