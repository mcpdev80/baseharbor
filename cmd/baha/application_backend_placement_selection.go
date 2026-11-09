package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	repositoryinspect "github.com/mcpdev80/baseharbor/internal/repositoryinspect"
)

// deploymentIdentityArgumentPrefix marks the optional variadic slot of
// resolvedRepositoryApplication as an explicit deployment-id selection rather
// than another command name. Deployment ids are UUIDs and can never collide
// with a command name, so the prefix keeps the two meanings apart (0427-175).
const deploymentIdentityArgumentPrefix = "deployment-id:"

// selectedDeploymentIDFromArguments reads an explicit deployment-id selection
// out of the optional variadic identity-exception slot. An empty selector adds
// nothing, so every other caller keeps the base behaviour (0427-175).
func selectedDeploymentIDFromArguments(command []string) (string, bool) {
	for _, arg := range command {
		if id, ok := strings.CutPrefix(arg, deploymentIdentityArgumentPrefix); ok {
			if id = strings.TrimSpace(id); id != "" {
				return id, true
			}
		}
	}
	return "", false
}

// identitySelectionArgument adapts an explicit deployment-id selection to the
// optional variadic slot of resolvedRepositoryApplication: the token never
// equals a command name, so the never-delivered discovery exception keeps
// reading command[0] as before. An empty selector contributes nothing and the
// call is then byte-for-byte the base one (0427-175).
func identitySelectionArgument(deploymentID string) string {
	if strings.TrimSpace(deploymentID) == "" {
		return ""
	}
	return deploymentIdentityArgumentPrefix + strings.TrimSpace(deploymentID)
}

// resolveApplicationEnvironmentSelecting is the shared resolver with one extra
// input: the deployment id the operator selected explicitly. An empty selector
// — every other command — keeps the behaviour below verbatim; what a selection
// changes is documented at each seam that reads it (0427-175).

// registeredRepositoryDeployment locates the deployment record of a repository
// application: through the (application_id, environment) inventory seam, or —
// when the command carries an explicit deployment-id selection (the
// uuid-addressed repair) — directly at the selected deployment path. A poisoned
// Target cannot answer the inventory question, because both twins claim that
// pair; the operator has named the record, so it is authoritative without any
// lookup, and the loader still proves the file there carries that target and
// deployment id (0427-175). A destroy of a locally initialized, never-persisted
// application keeps its discovery exception, resolved from the application
// identity instead.
func registeredRepositoryDeployment(ctx context.Context, target deployment.ResolvedTarget, targetRoot string, m application.Manifest, command []string) (deployment.DeploymentRecord, bool, error) {
	if selectedID, ok := selectedDeploymentIDFromArguments(command); ok {
		chosen, err := deployment.LoadDeploymentRecord(deployment.DeploymentIdentity{Target: target.Name, DeploymentID: selectedID})
		if err != nil {
			return deployment.DeploymentRecord{}, false, err
		}
		return chosen, true, nil
	}
	existing, found, err := deployment.FindDeployment(target.Name, m.ApplicationID, m.Environment)
	if err != nil && eligibleNeverDeliveredLocalDestroy(target, targetRoot, m, firstCommandName(command), err) {
		return deployment.DeploymentRecord{}, false, nil
	}
	return existing, found, err
}

// eligibleNeverDeliveredLocalDestroy recognizes only an independently
// initialized, never-persisted local repository application. It is NOT a Core
// owner grant: no LoadDeploymentRecord or SaveDeploymentRecord check is
// bypassed. A foreign/corrupt Core installation and any existing deployment
// namespace remain fail-closed. A valid one-shot proof is required before
// discovery. The uuid-addressed repair never relies on it: that path loads the
// selected record instead of looking one up (0427-175).
func eligibleNeverDeliveredLocalDestroy(target deployment.ResolvedTarget, targetRoot string, m application.Manifest, command string, ownerErr error) bool {
	if command != "destroy" || !errors.Is(ownerErr, deployment.ErrDeploymentOwnerMissing) ||
		target.TenantID != "" || target.AccessProvider != "local" || target.AccessReference != "local" ||
		(target.Scope != "local" && !(target.Name == "local" && target.Scope == "default")) {
		return false
	}
	deployments := filepath.Join(targetRoot, "deployments")
	if info, err := os.Lstat(deployments); err == nil {
		// Initialization may leave an empty namespace; a symlink or a
		// populated namespace is never a never-delivered proof. The per-app
		// lifecycle lock lives at <root>/deployments/.<id>.app_installation.lock
		// (a hidden dotfile, not a deployment directory); dotfiles are ignored
		// here so a lock file is never read as a persisted deployment (card
		// 0427-176).
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		entries, readErr := os.ReadDir(deployments)
		if readErr != nil {
			return false
		}
		nonDot := 0
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			nonDot++
		}
		if nonDot != 0 {
			return false
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false
	}
	return deployment.VerifyNeverDeliveredProof(deployment.DeploymentIdentity{
		ApplicationID: m.ApplicationID,
		Target:        target.Name,
		Application:   m.Name,
		Environment:   m.Environment,
	}) == nil
}

// appliedIntentOfRecord decodes the deployment-scoped intent snapshot the
// record itself carries (0427-175): the same projection the shared source branch
// yields for a healthy registry, taken from the record the operator named.
func appliedIntentOfRecord(record deployment.DeploymentRecord) (application.Manifest, error) {
	if len(record.Applied.Intent) == 0 {
		return application.Manifest{}, fmt.Errorf("deployment %s/%s/%s has no applied intent snapshot", record.Identity.Target, record.Identity.Application, record.Identity.Environment)
	}
	var m application.Manifest
	if err := json.Unmarshal(record.Applied.Intent, &m); err != nil {
		return application.Manifest{}, fmt.Errorf("decode applied intent for %s/%s/%s: %w", record.Identity.Target, record.Identity.Application, record.Identity.Environment, err)
	}
	if m.ApplicationID == "" {
		m.ApplicationID = record.Identity.ApplicationID
	}
	return m, nil
}

// discoverAuthoredWorkload runs the authored Compose discovery the lifecycle
// intent path performs for every caller, so a manifest taken from a record
// equals the one the repository source proves (0427-175).
func discoverAuthoredWorkload(ctx context.Context, repositoryRoot string, m application.Manifest) (application.Manifest, error) {
	if application.HasExplicitWorkload(m) {
		return m, m.Validate()
	}
	inspection, err := repositoryinspect.Inspect(ctx, repositoryRoot)
	if err != nil {
		return application.Manifest{}, err
	}
	if inspection.SelectedWorkloadSource != nil && inspection.SelectedWorkloadSource.Kind == repositoryinspect.WorkloadSourceCompose {
		analysis, err := repositoryinspect.AnalyzeComposeFile(repositoryRoot, inspection.SelectedWorkloadSource.Path)
		if err != nil {
			return application.Manifest{}, err
		}
		if len(analysis.WorkloadServices) > 0 {
			m = application.WithWorkloadComponents(m, analysis.WorkloadServices...)
		}
	}
	if err := m.Validate(); err != nil {
		return application.Manifest{}, &machine.Error{Code: machine.ErrorInvalidWorkload, CauseCode: "workload_missing", Message: "Application has no supported workload or runtime service.", Next: "Add a supported Compose source and run baha app init.", Cause: err}
	}
	return m, nil
}

// recordSourceApplication hydrates a located record from its registered
// repository source. Without a selection it is the shared path verbatim.
//
// With a selection (the uuid-addressed repair) the identity question is settled
// by the RECORD instead: reconciliation would prove this repository against
// every registration the registry can find, and on a poisoned Target the twins
// share application_id, environment, NAME and this very repository, so it must
// fail closed for them — including for the command that refusal advertises as
// its own repair (0427-175). The record's applied intent, layered with the same
// authored Compose discovery, is the projection the shared path yields for a
// healthy registry. Neither branch writes to disk; the destructive removal
// rechecks the deployment id.
func recordSourceApplication(ctx context.Context, target deployment.ResolvedTarget, resolved resolvedApplication, record deployment.DeploymentRecord, selection application.RepositoryEnvironmentSelection, command, selectedDeploymentID string) (resolvedApplication, error) {
	if selected := strings.TrimSpace(selectedDeploymentID); selected != "" {
		if resolved.DeploymentIdentity.DeploymentID != selected {
			// The located record is not the selected one: refuse before any
			// state of a sibling can be read or mutated (0427-175).
			return resolvedApplication{}, fmt.Errorf("deployment %s did not route the resolution: %s was resolved", selected, resolved.DeploymentIdentity.DeploymentID)
		}
		m, err := appliedIntentOfRecord(record)
		if err != nil {
			return resolvedApplication{}, err
		}
		if m, err = discoverAuthoredWorkload(ctx, selection.RepositoryRoot, m); err != nil {
			return resolvedApplication{}, err
		}
		resolved.Manifest, resolved.ManifestPath = m, selection.ManifestPath
		resolved.RepositoryRoot, resolved.FromRepository = selection.RepositoryRoot, true
		return resolved, nil
	}
	// Authored intent never supplies identity. This read projects the ID from
	// the existing authoritative deployment; it allocates no state.
	if selection.Manifest.ApplicationID == "" {
		selection.Manifest.ApplicationID = record.Identity.ApplicationID
	}
	if selection.Manifest.ApplicationID != record.Identity.ApplicationID {
		return resolvedApplication{}, fmt.Errorf(
			"registered source resolves application_id %q, expected %q",
			selection.Manifest.ApplicationID,
			record.Identity.ApplicationID,
		)
	}
	selection, err := resolveRepositoryLifecycleIntent(ctx, target, selection, command)
	if err != nil {
		return resolvedApplication{}, err
	}
	resolved.DeploymentIdentity.Application = selection.Manifest.Name
	resolved.Manifest = selection.Manifest
	resolved.ManifestPath = selection.ManifestPath
	resolved.RepositoryRoot = selection.RepositoryRoot
	resolved.FromRepository = true
	return resolved, nil
}

func appliedBackendSelection(resolved resolvedApplication, selectedDeploymentID string, command ...string) (application.Manifest, error) {
	if strings.TrimSpace(selectedDeploymentID) == "" {
		return resolveAppliedBackendSelection(resolved, command...)
	}
	return resolveAppliedBackendSelectionForDeployment(resolved, strings.TrimSpace(selectedDeploymentID), command...)
}

// resolveAppliedBackendSelectionForDeployment is the targeted twin of
// resolveAppliedBackendSelection: it performs the SAME backend-placement and
// desired-preference projection, but reads every deployment fact from the one
// deployment record the operator selected by uuid (0427-175).
//
// The untargeted variant reaches the same facts through the shared inventory
// seams, which identify a deployment by (application_id, environment). On a
// registry poisoned by two records claiming one application_id that pair
// matches BOTH twins, so the seam refuses — and the command the refusal itself
// advertises as the repair would abort before it could retire the duplicate.
// Carrying the selection through this projection keeps identity, applied intent
// and observed placement bound to the record that is about to be destroyed,
// and to nothing else.
func resolveAppliedBackendSelectionForDeployment(resolved resolvedApplication, deploymentUUID string, command ...string) (application.Manifest, error) {
	// The selected deployment directory is addressed by id alone, so the record
	// is read exactly where the operator pointed; the loader still proves the
	// file there carries that target and deployment id. The caller's identity
	// parts are NOT compared: on a poisoned registry the application_id matches
	// BOTH twins by definition, so a caller-side comparison would fail closed
	// even for the correct selection and the advertised repair could never
	// retire the duplicate (0427-175).
	record, err := deployment.LoadDeploymentRecordForRepair(resolved.Target.Name, deploymentUUID)
	if err != nil {
		return application.Manifest{}, err
	}
	// The record at the selected path is the only identity this projection
	// trusts. If it is not the deployment the operator named, the selection
	// routed somewhere else and the repair must stop here (0427-175).
	if !deployment.DeploymentRecordIdentityIs(record, resolved.Target.Name, deploymentUUID) {
		return application.Manifest{}, fmt.Errorf("deployment %s does not hold the selected record on target %s", deploymentUUID, resolved.Target.Name)
	}
	return appliedBackendPlacementProjectionForRecord(resolved, record, command...)
}

// appliedBackendPlacementProjectionForRecord mirrors
// resolveAppliedBackendSelection with the deployment record supplied directly
// instead of looked up by (application_id, environment); the retained-owner read,
// the placement resolution and the desired-preference layering stay the same
// production seams (0427-175).
func appliedBackendPlacementProjectionForRecord(resolved resolvedApplication, record deployment.DeploymentRecord, command ...string) (application.Manifest, error) {
	// The desired preferences of the SELECTED record replace the registry lookup
	// the shared reader would otherwise perform, so this projection reads no
	// ambiguous (application_id, environment) pair at all. A record without any
	// keeps nil, which means "inherit" exactly as in the shared path.
	m := application.WithProviderPreferences(resolved.Manifest, decodeDesiredProviderPreferences(record))
	projectDesired := func(value application.Manifest) (application.Manifest, error) {
		return application.WithDesiredProviderPlacementsAt(resolved.TargetStateRoot, value)
	}
	if resolved.TargetStateRoot == "" || (!m.Services.SQL && !m.Services.Cache && !m.Services.KeyValue) {
		return projectDesired(m)
	}
	applied, err := retainedBackendPlacementProjection(resolved, m, record.Applied.GeneratedState["backend_placements"])
	if err != nil {
		return application.Manifest{}, err
	}
	placements := map[capability.ProviderKind]capability.ProviderPlacement{}
	for _, kind := range []capability.ProviderKind{capability.ProviderPostgreSQL, capability.ProviderValkey} {
		if (kind == capability.ProviderPostgreSQL && !m.Services.SQL) || (kind == capability.ProviderValkey && !m.Services.Cache && !m.Services.KeyValue) {
			continue
		}
		explicit, err := application.HasExplicitProviderPlacement(m, kind)
		if err != nil {
			return application.Manifest{}, err
		}
		placement, err := application.ResolveProviderPlacement(applied, kind)
		if explicit {
			placement, err = application.ResolveConfiguredProviderPlacement(m, kind)
		}
		if err != nil {
			return application.Manifest{}, err
		}
		placements[kind] = placement
	}
	if len(placements) == 0 {
		return projectDesired(m)
	}
	projected, err := application.WithBackendPlacementProjection(m, placements)
	if err != nil {
		return application.Manifest{}, err
	}
	return projectDesired(projected)
}

// decodeDesiredProviderPreferences reads the deployment-scoped desired
// preferences exactly as the shared reader does, so a record without any keeps
// the registry lookup available to the projection (nil means "inherit").
func decodeDesiredProviderPreferences(record deployment.DeploymentRecord) []application.ProviderPreference {
	if len(record.DesiredProviderPreferences) == 0 {
		return nil
	}
	var preferences []application.ProviderPreference
	decoder := json.NewDecoder(bytes.NewReader(record.DesiredProviderPreferences))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&preferences); err != nil {
		return nil
	}
	if decoder.More() {
		return nil
	}
	return preferences
}

// firstCommandName returns the caller's command name for the discovery
// exceptions; a selection marker is always appended after it, so command[0]
// keeps its meaning for every caller (0427-175).
func firstCommandName(command []string) string {
	if len(command) == 0 {
		return ""
	}
	return command[0]
}
