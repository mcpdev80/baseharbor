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
func selectedRemoteSourceModel(applicationName, environment string) (*development.SourceModel, error) {
	manifestPath, err := application.FindRepositoryManifest(".")
	if err != nil {
		if errors.Is(err, application.ErrRepositoryManifestNotFound) {
			return nil, nil
		}
		return nil, err
	}
	local, err := application.LoadManifestFile(manifestPath)
	if err != nil {
		return nil, err
	}
	if local.Name != applicationName || local.Environment != environment {
		// Explicit NAME can intentionally address a different already
		// registered remote application. The unrelated local SourceModel
		// must not silently override the selected Core-owned application.
		return nil, nil
	}
	sourceFile := filepath.Join(filepath.Dir(manifestPath), filepath.FromSlash(development.SourceModelRelativePath))
	if _, err := os.Stat(sourceFile); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	model, _, err := development.LoadSourceModel(manifestPath)
	if err != nil {
		return nil, err
	}
	if model.Application != applicationName {
		return nil, fmt.Errorf("remote source model application %q does not match selected application %q", model.Application, applicationName)
	}
	if err := development.ValidateImmutableRemoteSources(model); err != nil {
		return nil, fmt.Errorf("remote application source is not immutable: %w", err)
	}
	return &model, nil
}

// Keep the read-only guard callable from existing tests and callers.
// The Core is the authority for artifact admission, not this local check.
func validateRemoteSelectedSources(applicationName, environment string) error {
	_, err := selectedRemoteSourceModel(applicationName, environment)
	return err
}
