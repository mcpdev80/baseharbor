package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

var applicationEnvironmentOverride string

func pushApplicationEnvironmentOverride(environment string) func() {
	previous := applicationEnvironmentOverride
	applicationEnvironmentOverride = strings.TrimSpace(environment)
	return func() { applicationEnvironmentOverride = previous }
}

func applyApplicationEnvironmentOverride(m application.Manifest) (application.Manifest, error) {
	if applicationEnvironmentOverride == "" {
		return m, nil
	}
	m.Environment = applicationEnvironmentOverride
	if err := m.Validate(); err != nil {
		return application.Manifest{}, fmt.Errorf("invalid --environment override: %w", err)
	}
	return m, nil
}

type resolvedApplication struct {
	Target               deployment.ResolvedTarget
	DeploymentIdentity   deployment.DeploymentIdentity
	DeploymentRecord     *deployment.DeploymentRecord
	Manifest             application.Manifest
	ManifestPath         string
	RepositoryRoot       string
	TargetStateRoot      string
	DeploymentStateRoot  string
	Store                application.Store
	SourceAvailable      bool
	FromRepository       bool
	IncompleteDeployment bool
}

func (r resolvedApplication) repositoryRoot() string {
	if r.RepositoryRoot != "" {
		return r.RepositoryRoot
	}
	if r.ManifestPath != "" {
		return filepath.Dir(r.ManifestPath)
	}
	if r.DeploymentRecord != nil {
		return r.DeploymentRecord.Source.Repository
	}
	return ""
}

func (r resolvedApplication) stateRoot() string {
	return r.DeploymentStateRoot
}

func resolveApplication(ctx context.Context, store application.Store, args []string, command string) (resolvedApplication, error) {
	ctx = localWorkloadScope(ctx, applicationBoundaryOperation(command))
	return resolveApplicationEnvironment(ctx, store, args, command, applicationEnvironmentOverride)
}

func resolveApplicationEnvironment(ctx context.Context, _ application.Store, args []string, command, environment string) (resolved resolvedApplication, err error) {
	defer func() {
		if err == nil {
			resolved.Manifest, err = resolveAppliedBackendSelection(resolved, command)
		}
	}()
	ctx = localWorkloadScope(ctx, applicationBoundaryOperation(command))
	filtered, argumentEnvironment, err := extractApplicationEnvironment(args, command)
	if err != nil {
		return resolvedApplication{}, err
	}
	if argumentEnvironment != "" {
		if environment != "" && environment != argumentEnvironment {
			return resolvedApplication{}, usageError("conflicting environment selections", "Specify the environment only once.")
		}
		environment = argumentEnvironment
	}
	args = filtered
	if len(args) > 1 {
		return resolvedApplication{}, usageError("baha app "+command+" accepts at most one NAME", "Run it without NAME inside an application repository, or pass NAME explicitly.")
	}

	target, err := effectiveTarget(ctx)
	if err != nil {
		return resolvedApplication{}, err
	}
	targetRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		return resolvedApplication{}, err
	}

	if len(args) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return resolvedApplication{}, fmt.Errorf("resolve current directory: %w", err)
		}
		selection, err := application.ResolveRepositoryEnvironment(cwd, environment)
		if err == nil {
			if authErr := ensureOperatorAuthForBoundary(ctx, target.Name, selection.Manifest.Environment); authErr != nil {
				return resolvedApplication{}, authErr
			}
			selection, err = resolveRepositoryLifecycleIntent(ctx, target, selection, command)
			if err != nil {
				return resolvedApplication{}, err
			}
			resolved, resolveErr := resolvedRepositoryApplication(target, targetRoot, selection, command)
			if resolveErr != nil {
				return resolvedApplication{}, resolveErr
			}
			if authErr := ensureOperatorAuthForBoundary(ctx, resolved.Target.Name, resolved.Manifest.Environment); authErr != nil {
				return resolvedApplication{}, authErr
			}
			return resolved, nil
		}
		if !errors.Is(err, application.ErrRepositoryManifestNotFound) {
			return resolvedApplication{}, err
		}
		return resolvedApplication{}, usageError(
			"no application deployment could be resolved",
			"Run inside a repository containing baseharbor.yaml, or pass an application NAME registered on the effective target.",
		)
	}

	// A --uuid selector routes directly at the registry seam, bypassing the
	// name filter entirely. It is the unambiguous identity for name-colliding
	// deployments (0427-149) that no -e value can disambiguate.
	deploymentUUID, uuidErr := extractApplicationUUID(args)
	if uuidErr != nil {
		return resolvedApplication{}, uuidErr
	}
	resolved, err = resolveRegisteredApplication(ctx, target, targetRoot, args[0], environment, command, deploymentUUID)
	if err != nil {
		return resolvedApplication{}, err
	}
	if err := ensureOperatorAuthForBoundary(ctx, resolved.Target.Name, resolved.Manifest.Environment); err != nil {
		return resolvedApplication{}, err
	}
	return resolved, nil
}

// eligibleNeverDeliveredLocalDestroy recognizes only an independently
// initialized, never-persisted local repository application. It is NOT a Core
// owner grant: no LoadDeploymentRecord or SaveDeploymentRecord check is bypassed.
// A foreign/corrupt Core installation and any existing deployment namespace
// remain fail-closed. A valid one-shot proof is required before discovery.
func eligibleNeverDeliveredLocalDestroy(target deployment.ResolvedTarget, targetRoot string, m application.Manifest, command string, ownerErr error) bool {
	if command != "destroy" || !errors.Is(ownerErr, deployment.ErrDeploymentOwnerMissing) ||
		target.TenantID != "" || target.AccessProvider != "local" || target.AccessReference != "local" ||
		(target.Scope != "local" && !(target.Name == "local" && target.Scope == "default")) {
		return false
	}
	deployments := filepath.Join(targetRoot, "deployments")
	if info, err := os.Lstat(deployments); err == nil {
		// Initialization may leave an empty namespace; a symlink or a
		// populated namespace is never a never-delivered proof.
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		entries, err := os.ReadDir(deployments)
		if err != nil || len(entries) != 0 {
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

func resolvedRepositoryApplication(target deployment.ResolvedTarget, targetRoot string, selection application.RepositoryEnvironmentSelection, command ...string) (resolvedApplication, error) {
	m := selection.Manifest
	existing, found, err := deployment.FindDeployment(target.Name, m.ApplicationID, m.Environment)
	// A locally initialized application may be destroyed before its first
	// deployment record exists. The one-shot initialization proof is consumed
	// BEFORE the first persistence, so it cannot authorize deletion of an
	// existing deployment. This exception changes only discovery for destroy:
	// storage read/write ownership checks still fail closed.
	if err != nil && len(command) > 0 && eligibleNeverDeliveredLocalDestroy(target, targetRoot, m, command[0], err) {
		found, err = false, nil
	}
	if err != nil {
		return resolvedApplication{}, err
	}

	var id deployment.DeploymentIdentity
	var record *deployment.DeploymentRecord
	if found {
		id = existing.Identity
		id.Application = m.Name
		existing.Identity = id
		record = &existing
	} else {
		id, err = deployment.NewDeploymentIdentity(target.Name, m.ApplicationID, m.Name, m.Environment)
		if err != nil {
			return resolvedApplication{}, err
		}
	}

	deploymentRoot, err := deployment.DeploymentRoot(id)
	if err != nil {
		return resolvedApplication{}, err
	}
	return resolvedApplication{
		Target:              target,
		DeploymentIdentity:  id,
		DeploymentRecord:    record,
		Manifest:            selection.Manifest,
		ManifestPath:        selection.ManifestPath,
		RepositoryRoot:      selection.RepositoryRoot,
		TargetStateRoot:     targetRoot,
		DeploymentStateRoot: deploymentRoot,
		Store:               application.Store{Root: filepath.Join(deploymentRoot, "state"), Namespace: target.Name},
		SourceAvailable:     true,
		FromRepository:      true,
	}, nil
}

func resolveRegisteredApplication(ctx context.Context, target deployment.ResolvedTarget, targetRoot, name, environment, command string, deploymentUUID string) (resolvedApplication, error) {
	records, warnings, err := deployment.ListDeploymentsForDisplay(target.Name)
	if err != nil {
		return resolvedApplication{}, err
	}
	// A --uuid selector resolves the exact deployment directory, bypassing the
	// name/environment filter. It is the only unambiguous identity for a
	// name-colliding deployment that no -e value can disambiguate (0427-149).
	if strings.TrimSpace(deploymentUUID) != "" {
		for _, record := range records {
			if record.Identity.DeploymentID == deploymentUUID {
				return resolveRecordByIdentity(ctx, target, targetRoot, record, command)
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
	var matches []deployment.DeploymentRecord
	for _, record := range records {
		if record.Identity.Application != name {
			continue
		}
		if environment != "" && record.Identity.Environment != environment {
			continue
		}
		matches = append(matches, record)
	}
	if len(matches) == 0 {
		var incomplete []resolvedApplication
		for _, warning := range warnings {
			stateErr, ok := deployment.DeploymentRecordState(warning)
			if !ok || stateErr.Kind != "incomplete" {
				continue
			}
			candidate, matched, resolveErr := resolveIncompleteRegisteredApplication(target, targetRoot, stateErr.Identity, name, environment, command)
			if resolveErr != nil {
				return resolvedApplication{}, resolveErr
			}
			if matched {
				incomplete = append(incomplete, candidate)
			}
		}
		if len(incomplete) == 1 {
			return incomplete[0], nil
		}
		if len(incomplete) > 1 {
			return resolvedApplication{}, &machine.Error{
				Code:        machine.ErrorOwnershipAmbiguous,
				CauseCode:   "AMBIGUOUS_INCOMPLETE_DEPLOYMENT",
				Message:     fmt.Sprintf("multiple incomplete deployments match application %q on target %q", name, target.Name),
				Resource:    target.Name + "/" + name,
				Next:        "Select an environment with -e/--environment, or inspect protected deployment state before cleanup.",
				Remediation: "operator_review",
			}
		}
		if environment == "" {
			return resolvedApplication{}, &machine.Error{
				Code:      machine.ErrorNotFound,
				CauseCode: "application_deployment_not_found",
				Message:   fmt.Sprintf("application %q has no registered deployment on target %q", name, target.Name),
				Resource:  target.Name + "/" + name,
				Next:      "Run 'baha app list' to inspect registered applications, or run the command inside the application repository.",
			}
		}
		return resolvedApplication{}, &machine.Error{
			Code:      machine.ErrorNotFound,
			CauseCode: "application_deployment_not_found",
			Message:   fmt.Sprintf("application %q environment %q has no registered deployment on target %q", name, environment, target.Name),
			Resource:  target.Name + "/" + name + "/" + environment,
			Next:      "Run 'baha app list' to inspect registered applications and environments.",
		}
	}
	if len(matches) > 1 {
		// Multiple registrations share this application NAME. When they span
		// distinct environments a plain -e/--environment disambiguates them.
		// When they share the SAME environment (a pre-existing name duplicate
		// that the registry now refuses to create anew, 0427-149) no -e value
		// can resolve the collision, so surface the concrete deployment uuids
		// and route the operator onto the uuid path — the only unambiguous
		// identity for edit/show/plan/destroy. This is the actionable
		// alternative to the former opaque "environment selection required".
		environments := make([]string, 0, len(matches))
		byEnv := make(map[string][]string)
		for _, record := range matches {
			environments = append(environments, record.Identity.Environment)
			byEnv[record.Identity.Environment] = append(byEnv[record.Identity.Environment], record.Identity.DeploymentID)
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
			Next:        "Select one deployment by uuid: baha app " + command + " --uuid <deployment-id>, or by a distinct environment with -e/--environment.",
			Remediation: "operator_review",
		}
	}
	record := matches[0]
	return resolveRecordByIdentity(ctx, target, targetRoot, record, command)
}

// resolveRecordByIdentity reconstructs a resolved application from a single
// authoritative deployment record. It is shared by the name filter (exact
// match) and the --uuid selector so both paths hydrate identical state and
// behave identically for a name-colliding deployment.
func resolveRecordByIdentity(ctx context.Context, target deployment.ResolvedTarget, targetRoot string, record deployment.DeploymentRecord, command string) (resolvedApplication, error) {
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
		if sourceErr == nil {
			if selection.Manifest.Name != record.Identity.Application || selection.Manifest.Environment != record.Identity.Environment {
				return resolvedApplication{}, fmt.Errorf("registered source does not match authoritative deployment name/environment")
			}
			// Authored intent never supplies identity. This read projects the ID
			// from the existing authoritative deployment; it allocates no state.
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
			selection, sourceErr = resolveRepositoryLifecycleIntent(ctx, target, selection, command)
			if sourceErr != nil {
				return resolvedApplication{}, sourceErr
			}
			resolved.DeploymentIdentity.Application = selection.Manifest.Name
			resolved.Manifest = selection.Manifest
			resolved.ManifestPath = selection.ManifestPath
			resolved.RepositoryRoot = selection.RepositoryRoot
			resolved.FromRepository = true
			return resolved, nil
		}
		resolved.SourceAvailable = false
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

func resolveIncompleteRegisteredApplication(
	target deployment.ResolvedTarget,
	targetRoot string,
	id deployment.DeploymentIdentity,
	name, environment, command string,
) (resolvedApplication, bool, error) {
	deploymentRoot, err := deployment.DeploymentRoot(id)
	if err != nil {
		return resolvedApplication{}, false, err
	}
	store := application.Store{Root: filepath.Join(deploymentRoot, "state"), Namespace: target.Name}
	manifests, err := store.List()
	if err != nil {
		return resolvedApplication{}, false, &machine.Error{
			Code:        machine.ErrorOwnershipAmbiguous,
			CauseCode:   "INCOMPLETE_DEPLOYMENT_STATE",
			Message:     fmt.Sprintf("incomplete deployment %s/%s cannot be reconstructed safely", id.Target, id.DeploymentID),
			Resource:    id.Target + "/" + id.DeploymentID,
			Next:        "Inspect the protected deployment state before cleanup.",
			Remediation: "operator_review",
			Cause:       err,
		}
	}

	var matches []application.Manifest
	for _, manifest := range manifests {
		if manifest.Name != name {
			continue
		}
		if environment != "" && manifest.Environment != environment {
			continue
		}
		matches = append(matches, manifest)
	}
	if len(matches) == 0 {
		if len(manifests) == 0 {
			return resolvedApplication{}, false, &machine.Error{
				Code:        machine.ErrorOwnershipAmbiguous,
				CauseCode:   "INCOMPLETE_DEPLOYMENT_STATE",
				Message:     fmt.Sprintf("incomplete deployment %s/%s has no protected application identity", id.Target, id.DeploymentID),
				Resource:    id.Target + "/" + id.DeploymentID,
				Next:        "Inspect the protected deployment state before cleanup.",
				Remediation: "operator_review",
			}
		}
		return resolvedApplication{}, false, nil
	}
	if len(matches) > 1 {
		return resolvedApplication{}, false, &machine.Error{
			Code:        machine.ErrorOwnershipAmbiguous,
			CauseCode:   "INCOMPLETE_DEPLOYMENT_IDENTITY_AMBIGUOUS",
			Message:     fmt.Sprintf("protected state for deployment %s/%s contains multiple matching application identities", id.Target, id.DeploymentID),
			Resource:    id.Target + "/" + id.DeploymentID,
			Next:        "Inspect the protected deployment state before cleanup.",
			Remediation: "operator_review",
		}
	}

	m := matches[0]
	id.ApplicationID = m.ApplicationID
	id.Application = m.Name
	id.Environment = m.Environment
	if err := id.Validate(); err != nil {
		return resolvedApplication{}, false, &machine.Error{
			Code:        machine.ErrorOwnershipAmbiguous,
			CauseCode:   "INCOMPLETE_DEPLOYMENT_IDENTITY_MISMATCH",
			Message:     fmt.Sprintf("protected application state does not provide a valid stable identity for deployment %s/%s", id.Target, id.DeploymentID),
			Resource:    id.Target + "/" + id.DeploymentID,
			Next:        "Inspect the protected deployment state before cleanup.",
			Remediation: "operator_review",
			Cause:       err,
		}
	}
	manifestPath := filepath.Join(store.Root, m.Name, "baseharbor.yaml")
	if commandRequiresLiveSource(command) {
		return resolvedApplication{}, false, &machine.Error{
			Code:      machine.ErrorSourceMissing,
			CauseCode: "INCOMPLETE_DEPLOYMENT_SOURCE",
			Message:   fmt.Sprintf("incomplete deployment %s/%s has no durable source record", id.Target, id.DeploymentID),
			Resource:  id.Target + "/" + id.DeploymentID,
			Next:      "Run the operation from the repository source so BaseHarbor can re-establish the deployment record.",
		}
	}
	return resolvedApplication{
		Target:               target,
		DeploymentIdentity:   id,
		Manifest:             m,
		ManifestPath:         manifestPath,
		TargetStateRoot:      targetRoot,
		DeploymentStateRoot:  deploymentRoot,
		Store:                store,
		SourceAvailable:      false,
		FromRepository:       false,
		IncompleteDeployment: true,
	}, true, nil
}

func commandRequiresLiveSource(command string) bool {
	switch command {
	case "apply", "update", "plan", "preflight", "init":
		return true
	default:
		return false
	}
}

func extractApplicationEnvironment(args []string, command string) ([]string, string, error) {
	filtered := make([]string, 0, len(args))
	environment := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-e" || arg == "--environment":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return nil, "", usageError("--environment requires ENV", "Example: baha app "+command+" -e prod")
			}
			i++
			value := strings.TrimSpace(args[i])
			if environment != "" && environment != value {
				return nil, "", usageError("conflicting environment selections", "Specify -e/--environment only once.")
			}
			environment = value
		case strings.HasPrefix(arg, "--environment="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--environment="))
			if value == "" {
				return nil, "", usageError("--environment requires ENV", "Example: baha app "+command+" --environment=prod")
			}
			if environment != "" && environment != value {
				return nil, "", usageError("conflicting environment selections", "Specify -e/--environment only once.")
			}
			environment = value
		case arg == "--uuid" || strings.HasPrefix(arg, "--uuid="):
			// The --uuid selector is consumed here: it is removed from the
			// positional args so it never counts toward the one-NAME limit and
			// never reaches the name filter. The value is captured separately by
			// extractApplicationUUID for the registered-application path.
			if arg == "--uuid" && i+1 >= len(args) {
				return nil, "", usageError("--uuid requires a deployment id", "Example: baha app "+command+" --uuid <deployment-id>")
			}
			if arg == "--uuid" {
				i++
			}
		default:
			filtered = append(filtered, arg)
		}
	}
	return filtered, environment, nil
}

// extractApplicationUUID pulls an optional --uuid <deployment-id> selector out
// of the raw args for the registered-application resolution path. It is the
// unambiguous identity for a name-colliding deployment that no -e value can
// disambiguate (0427-149). Only valid against the registered-application path;
// a repository/cwd command rejects it. A malformed uuid is rejected so the
// operator cannot accidentally resolve the wrong deployment.
func extractApplicationUUID(args []string) (string, error) {
	uuid := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--uuid":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", fmt.Errorf("--uuid requires a deployment id")
			}
			i++
			value := strings.TrimSpace(args[i])
			if uuid != "" && uuid != value {
				return "", usageError("conflicting deployment id selections", "Specify --uuid only once.")
			}
			uuid = value
		case strings.HasPrefix(arg, "--uuid="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--uuid="))
			if uuid != "" && uuid != value {
				return "", usageError("conflicting deployment id selections", "Specify --uuid only once.")
			}
			uuid = value
		}
	}
	return uuid, nil
}

func configureRepositoryComposeEnvironment(repoRoot, stateRoot string) error {
	if strings.TrimSpace(os.Getenv("COMPOSE_ENV_FILES")) != "" {
		return nil
	}
	var paths []string
	for _, path := range []string{
		repositoryInitEnvPathFromStateRoot(stateRoot),
		filepath.Join(repoRoot, ".env"),
	} {
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect repository Compose environment file: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("repository Compose environment file %s is not a regular file", path)
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil
	}
	if err := os.Setenv("COMPOSE_ENV_FILES", strings.Join(paths, ",")); err != nil {
		return fmt.Errorf("configure repository Compose environment files: %w", err)
	}
	return nil
}

func checkManifestPermissions(path string, fromRepository bool) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if fromRepository {
		if info.Mode().Perm()&0o002 != 0 {
			return fmt.Errorf("repository manifest %s is writable by others (%o)", path, info.Mode().Perm())
		}
		return nil
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s is accessible by group or others (%o)", path, info.Mode().Perm())
	}
	return nil
}
