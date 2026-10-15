package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// Released bindings and incomplete Core modules must not release retained
// consumer authority. This inspection also applies when compose.yaml is absent.
func observeDestroyConsumerAuthority(ctx context.Context, operation string, target deployment.ResolvedTarget, owners map[string]string) error {
	root, err := targetDataRoot(target)
	if err != nil {
		return err
	}
	path := filepath.Join(root, "provider-registry.json")
	data, err := runtimeMutationOwnerBytes(path, true)
	if err != nil {
		return err
	}
	owners[path] = runtimeMutationRevision(data)
	zeroBytes(data)
	registry, err := (capability.RegistryStore{Path: path}).Load()
	if err != nil {
		return invalidProviderConsumerInventory(err)
	}
	for _, binding := range registry.Bindings {
		if err := authorizeMCPOperation(ctx, operation, target.Name, binding.Environment, binding.ApplicationID, ""); err != nil {
			return err
		}
	}
	for _, instance := range registry.Instances {
		if instance.Scope == capability.ScopeApplication {
			if err := authorizeMCPOperation(ctx, operation, target.Name, instance.OwnerEnvironment, instance.OwnerApplicationID, ""); err != nil {
				return err
			}
		}
	}
	runtimeRoot, err := targetRuntimeStateRoot(target)
	if err != nil {
		return err
	}
	statePath := filepath.Join(runtimeRoot, "installation.json")
	data, err = runtimeMutationOwnerBytes(statePath, true)
	if err != nil {
		return err
	}
	owners[statePath] = runtimeMutationRevision(data)
	zeroBytes(data)
	state, err := coreinstallation.Load(runtimeRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return invalidProviderConsumerInventory(err)
	}
	if state.Spec.Target != target.Name || state.Spec.Runtime != target.RuntimeProvider {
		return invalidProviderConsumerInventory(errors.New("installation target/runtime changed"))
	}
	environment := "dev"
	if state.Defaults.ProtectedManagement {
		environment = "prod"
	}
	return authorizeMCPOperation(ctx, operation, target.Name, environment, state.ID, "")
}
