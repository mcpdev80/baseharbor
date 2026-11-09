package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/development"
)

// validateRemoteSelectedSources prevents a local workspace's mutable source
// declaration from being treated as an immutable remote release. The source
// model is optional for an already Core-managed app: it does not grant identity
// or itself publish a source candidate to Core.
func validateRemoteSelectedSources(applicationName, environment string) error {
	manifestPath, err := application.FindRepositoryManifest(".")
	if err != nil {
		if errors.Is(err, application.ErrRepositoryManifestNotFound) {
			return nil
		}
		return err
	}
	local, err := application.LoadManifestFile(manifestPath)
	if err != nil {
		return err
	}
	if local.Name != applicationName || local.Environment != environment {
		// Explicit NAME can intentionally address a different already
		// registered remote application. The unrelated local SourceModel
		// must not silently override the selected Core-owned application.
		return nil
	}
	sourceFile := filepath.Join(filepath.Dir(manifestPath), filepath.FromSlash(development.SourceModelRelativePath))
	if _, err := os.Stat(sourceFile); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	model, _, err := development.LoadSourceModel(manifestPath)
	if err != nil {
		return err
	}
	if err := development.ValidateImmutableRemoteSources(model); err != nil {
		return fmt.Errorf("remote application source is not immutable: %w", err)
	}
	return nil
}
