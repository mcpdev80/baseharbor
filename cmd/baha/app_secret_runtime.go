package main

import (
	"context"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func resolvedApplicationSecretService(ctx context.Context, resolved resolvedApplication) (*applicationsecret.Service, error) {
	compose, err := detectComposeForApplication(ctx, resolved, bhruntime.CapabilityServiceExec)
	if err != nil {
		return nil, err
	}
	platformFiles, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		return nil, err
	}
	runtimeFiles, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		return nil, err
	}
	return applicationsecret.NewForApplicationRuntime(resolved.Store, compose, platformFiles, resolved.Manifest, runtimeFiles), nil
}
