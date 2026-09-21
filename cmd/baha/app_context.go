package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
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
	Manifest       application.Manifest
	ManifestPath   string
	RepositoryRoot string
	StateRoot      string
	Store          application.Store
	FromRepository bool
}

func (r resolvedApplication) repositoryRoot() string {
	if r.RepositoryRoot != "" {
		return r.RepositoryRoot
	}
	if r.ManifestPath != "" {
		return filepath.Dir(r.ManifestPath)
	}
	return ""
}

func (r resolvedApplication) stateRoot() string {
	if r.StateRoot != "" {
		return r.StateRoot
	}
	root := r.repositoryRoot()
	if root == "" {
		return ""
	}
	return filepath.Join(root, ".baseharbor")
}

func resolveApplication(store application.Store, args []string, command string) (resolvedApplication, error) {
	if len(args) > 1 {
		return resolvedApplication{}, usageError("baha app "+command+" accepts at most one NAME", "Run it without NAME inside an application repository, or pass NAME explicitly.")
	}
	if len(args) == 1 {
		m, path, err := store.Load(args[0])
		if err != nil {
			return resolvedApplication{}, err
		}
		if applicationEnvironmentOverride != "" && m.Environment != applicationEnvironmentOverride {
			return resolvedApplication{}, usageError(
				"--environment cannot retarget stored application state",
				"Run inside the application repository so BaseHarbor can select the environment-scoped manifest and state safely.",
			)
		}
		return resolvedApplication{Manifest: m, ManifestPath: path, Store: store}, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return resolvedApplication{}, fmt.Errorf("resolve current directory: %w", err)
	}
	selection, err := application.ResolveRepositoryEnvironment(cwd, applicationEnvironmentOverride)
	if err != nil {
		return resolvedApplication{}, usageError(
			"no application environment could be resolved",
			"Run inside a repository containing baseharbor.yaml or envs/<environment>/baseharbor.yaml; use -e/--environment when multiple environments exist.",
		)
	}

	stateRoot := application.RepositoryEnvironmentStateRoot(selection)
	if err := configureRepositoryComposeEnvironment(selection.RepositoryRoot, stateRoot); err != nil {
		return resolvedApplication{}, err
	}
	repoStore := application.Store{Root: filepath.Join(stateRoot, "apps")}
	if _, err := repoStore.Sync(selection.Manifest); err != nil {
		return resolvedApplication{}, fmt.Errorf("synchronize repository manifest: %w", err)
	}
	return resolvedApplication{
		Manifest:       selection.Manifest,
		ManifestPath:   selection.ManifestPath,
		RepositoryRoot: selection.RepositoryRoot,
		StateRoot:      stateRoot,
		Store:          repoStore,
		FromRepository: true,
	}, nil
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
