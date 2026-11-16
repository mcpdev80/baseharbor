package main

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

type secretMutationResult struct {
	Application string `json:"application"`
	Environment string `json:"environment"`
	Key         string `json:"key"`
	Updated     bool   `json:"updated,omitempty"`
	Deleted     bool   `json:"deleted,omitempty"`
	Preview     bool   `json:"preview,omitempty"`
}

func authorizeApplicationOperation(ctx context.Context, id string, resolved resolvedApplication) error {
	return authorizeMCPOperation(ctx, id, resolved.Target.Name, resolved.Manifest.Environment, resolved.Manifest.ApplicationID, resolved.ManifestPath)
}

// authorizeHumanReveal gates any path that would return a managed secret value
// to a human operator (CLI --reveal, app creds --reveal). It lives on the
// separate human-authorization seam enforced by
// operatorauth.RequireHumanOperator: a verified operator principal is mandatory,
// and the local-runtime/workload authority and trusted-local fallback that cover
// ordinary application operations can never satisfy it. Workload reads that do
// not reveal keep using their existing operation (app.environment/app.connection)
// and are unaffected. The distinct secret.reveal machine operation id in the
// machine contract documents this seam; the runtime enforcement is the explicit,
// fail-closed RequireHumanOperator gate (owner: mechanism B, 057).
func authorizeHumanReveal(ctx context.Context, target, environment, application string) error {
	return operatorauth.RequireHumanOperator(ctx)
}
func listApplicationSecrets(ctx context.Context, resolved resolvedApplication) ([]applicationsecret.Metadata, error) {
	if err := authorizeApplicationOperation(ctx, "secret.list", resolved); err != nil {
		return nil, err
	}
	service, err := resolvedApplicationSecretService(ctx, resolved)
	if err != nil {
		return nil, err
	}
	items, err := service.List(ctx, resolved.Manifest.Name)
	if err != nil {
		return nil, err
	}
	configured := make([]applicationsecret.Metadata, 0, len(items))
	for _, item := range items {
		if item.Present {
			configured = append(configured, item)
		}
	}
	return configured, nil
}

// Input is read only after shared authorization and secret-scope validation.
// The callback keeps protected bytes out of transport DTOs and audit metadata.
func setApplicationSecret(ctx context.Context, resolved resolvedApplication, key string, input func() ([]byte, error)) (secretMutationResult, error) {
	if err := authorizeApplicationOperation(ctx, "secret.set", resolved); err != nil {
		return secretMutationResult{}, err
	}
	service, err := resolvedApplicationSecretService(ctx, resolved)
	if err != nil {
		return secretMutationResult{}, err
	}
	value, err := input()
	if err != nil {
		return secretMutationResult{}, err
	}
	defer zeroBytes(value)
	if err := service.Set(ctx, resolved.Manifest.Name, key, value); err != nil {
		return secretMutationResult{}, err
	}
	return secretMutationResult{Application: resolved.Manifest.Name, Environment: resolved.Manifest.Environment, Key: key, Updated: true}, nil
}
func deleteApplicationSecret(ctx context.Context, resolved resolvedApplication, key string, confirmed bool) (secretMutationResult, error) {
	if err := authorizeApplicationOperation(ctx, "secret.delete", resolved); err != nil {
		return secretMutationResult{}, err
	}
	service, err := resolvedApplicationSecretService(ctx, resolved)
	if err != nil {
		return secretMutationResult{}, err
	}
	items, err := service.List(ctx, resolved.Manifest.Name)
	if err != nil {
		return secretMutationResult{}, err
	}
	found := false
	for _, item := range items {
		if item.Name == key && item.Present {
			found = true
		}
	}
	if !found {
		return secretMutationResult{}, openbao.ErrApplicationSecretNotFound
	}
	result := secretMutationResult{Application: resolved.Manifest.Name, Environment: resolved.Manifest.Environment, Key: key, Preview: !confirmed}
	if confirmed {
		if err := service.Delete(ctx, resolved.Manifest.Name, key); err != nil {
			return result, err
		}
		result.Deleted = true
	}
	return result, nil
}
