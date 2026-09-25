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
	Target              deployment.ResolvedTarget
	DeploymentIdentity  deployment.DeploymentIdentity
	DeploymentRecord    *deployment.DeploymentRecord
	Manifest            application.Manifest
	ManifestPath        string
	RepositoryRoot      string
	TargetStateRoot     string
	DeploymentStateRoot string
	Store               application.Store
	SourceAvailable     bool
	FromRepository      bool
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
	return resolveApplicationEnvironment(ctx, store, args, command, applicationEnvironmentOverride)
}

func resolveApplicationEnvironment(ctx context.Context, _ application.Store, args []string, command, environment string) (resolvedApplication, error) {
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
			return resolvedRepositoryApplication(target, targetRoot, selection)
		}
		if !errors.Is(err, application.ErrRepositoryManifestNotFound) {
			return resolvedApplication{}, err
		}
		return resolvedApplication{}, usageError(
			"no application deployment could be resolved",
			"Run inside a repository containing baseharbor.yaml, or pass an application NAME registered on the effective target.",
		)
	}

	return resolveRegisteredApplication(target, targetRoot, args[0], environment, command)
}

func resolvedRepositoryApplication(target deployment.ResolvedTarget, targetRoot string, selection application.RepositoryEnvironmentSelection) (resolvedApplication, error) {
	id := deployment.DeploymentIdentity{
		Target:      target.Name,
		Application: selection.Manifest.Name,
		Environment: selection.Manifest.Environment,
	}
	deploymentRoot, err := deployment.DeploymentRoot(id)
	if err != nil {
		return resolvedApplication{}, err
	}
	var record *deployment.DeploymentRecord
	if existing, err := deployment.LoadDeploymentRecord(id); err == nil {
		record = &existing
	} else if !errors.Is(err, os.ErrNotExist) {
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

func resolveRegisteredApplication(target deployment.ResolvedTarget, targetRoot, name, environment, command string) (resolvedApplication, error) {
	records, err := deployment.ListDeployments(target.Name)
	if err != nil {
		return resolvedApplication{}, err
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
		if environment == "" {
			return resolvedApplication{}, fmt.Errorf("application %q has no registered deployment on target %q", name, target.Name)
		}
		return resolvedApplication{}, fmt.Errorf("application %q environment %q has no registered deployment on target %q", name, environment, target.Name)
	}
	if len(matches) > 1 {
		return resolvedApplication{}, usageError(
			"environment selection required for application "+name,
			"Select one registered environment with -e/--environment.",
		)
	}
	record := matches[0]
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
			if selection.Manifest.Name != record.Identity.Application {
				return resolvedApplication{}, fmt.Errorf("registered source resolves application %q, expected %q", selection.Manifest.Name, record.Identity.Application)
			}
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
		default:
			filtered = append(filtered, arg)
		}
	}
	return filtered, environment, nil
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
