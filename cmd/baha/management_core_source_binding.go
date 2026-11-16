package main

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/mcpdev80/baseharbor/internal/development"
)

// bindRegisteredCoreSource protects the source chosen during initial
// Core-managed Application creation. An apply without a source_model may
// select that stored, immutable source; it must never fall back to the
// empty managed intent. A later workstation selection cannot silently
// change source ownership or revision.
func bindRegisteredCoreSource(resolved resolvedApplication, supplied *development.SourceModel) (*development.SourceModel, error) {
	if resolved.DeploymentRecord == nil || resolved.DeploymentRecord.Source.Kind != "managed" {
		return supplied, nil
	}
	if resolved.ManifestPath == "" {
		return nil, fmt.Errorf("Core-managed source has no authoritative manifest path")
	}
	stored, _, err := development.LoadSourceModel(resolved.ManifestPath)
	if errors.Is(err, development.ErrWorkspaceModelMissing) {
		if supplied != nil {
			return nil, fmt.Errorf("source was supplied for an existing managed application without a Core-registered source; explicit source admission is required")
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("Core-managed source selection is invalid: %w", err)
	}
	if stored.Application != resolved.Manifest.Name {
		return nil, fmt.Errorf("stored Core source selection belongs to another application")
	}
	if err := development.ValidateImmutableRemoteSources(stored); err != nil {
		return nil, fmt.Errorf("stored Core source selection is not immutable: %w", err)
	}
	if supplied != nil && !reflect.DeepEqual(*supplied, stored) {
		return nil, fmt.Errorf("remote source selection differs from the authoritative Core registration")
	}
	return &stored, nil
}

// rejectUnboundRegisteredSourceMutation prevents update/repair from bypassing
// the immutable source's approval and transport transaction by reconstructing
// a legacy managed runtime. Destructive cleanup remains a separate operation.
func rejectUnboundRegisteredSourceMutation(resolved resolvedApplication, operation string) error {
	source, err := bindRegisteredCoreSource(resolved, nil)
	if err != nil {
		return err
	}
	if source == nil {
		return nil
	}
	return fmt.Errorf("Core-managed immutable Git/OCI source cannot be %s until its confirmed candidate is bound to connector delivery", operation)
}
