package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
)

type resolvedApplication struct {
	Manifest       application.Manifest
	ManifestPath   string
	Store          application.Store
	FromRepository bool
}

func resolveApplication(store application.Store, args []string, command string) (resolvedApplication, error) {
	if len(args) > 1 {
		return resolvedApplication{}, usageError("baha app "+command+" accepts at most one NAME", "Run it without NAME inside a repository containing baseharbor.yaml, or pass NAME explicitly.")
	}
	if len(args) == 1 {
		m, path, err := store.Load(args[0])
		if err != nil {
			return resolvedApplication{}, err
		}
		return resolvedApplication{Manifest: m, ManifestPath: path, Store: store}, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return resolvedApplication{}, fmt.Errorf("resolve current directory: %w", err)
	}
	path, err := application.FindRepositoryManifest(cwd)
	if err != nil {
		return resolvedApplication{}, usageError("no application NAME was provided and no baseharbor.yaml was found", "Run this command inside an application repository or pass NAME explicitly.")
	}
	m, err := application.LoadManifestFile(path)
	if err != nil {
		return resolvedApplication{}, err
	}
	repoRoot := filepath.Dir(path)
	if err := configureRepositoryComposeEnvironment(repoRoot); err != nil {
		return resolvedApplication{}, err
	}
	repoStore := application.Store{Root: filepath.Join(repoRoot, ".baseharbor", "apps")}
	if _, err := repoStore.Sync(m); err != nil {
		return resolvedApplication{}, fmt.Errorf("synchronize repository manifest: %w", err)
	}
	return resolvedApplication{Manifest: m, ManifestPath: path, Store: repoStore, FromRepository: true}, nil
}

func configureRepositoryComposeEnvironment(repoRoot string) error {
	if strings.TrimSpace(os.Getenv("COMPOSE_ENV_FILES")) != "" {
		return nil
	}
	path := filepath.Join(repoRoot, ".env")
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect repository Compose environment file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("repository Compose environment file %s is not a regular file", path)
	}
	if err := os.Setenv("COMPOSE_ENV_FILES", path); err != nil {
		return fmt.Errorf("configure repository Compose environment file: %w", err)
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
