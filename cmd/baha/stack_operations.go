package main

import (
	"context"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

type stackShowResult struct {
	Scope   development.ProfileScope `json:"scope"`
	Path    string                   `json:"path"`
	Sources []string                 `json:"sources"`
	Profile development.StackProfile `json:"profile"`
}
type stackCreateResult struct {
	Name  string                   `json:"name"`
	Scope development.ProfileScope `json:"scope"`
	Path  string                   `json:"path"`
}

func listStackProfiles(ctx context.Context) ([]development.ProfileEntry, error) {
	if err := authorizeCurrentMCPContext(ctx, "stack.list", "", "", ""); err != nil {
		return nil, err
	}
	_, catalog, err := stackCatalog()
	if err != nil {
		return nil, err
	}
	entries := make([]development.ProfileEntry, 0, len(catalog))
	for _, name := range sortedProfileNames(catalog) {
		entries = append(entries, catalog[name])
	}
	return entries, nil
}
func inspectStackProfile(ctx context.Context, name string) (stackShowResult, error) {
	if err := authorizeCurrentMCPContext(ctx, "stack.show", "", "", ""); err != nil {
		return stackShowResult{}, err
	}
	_, catalog, err := stackCatalog()
	if err != nil {
		return stackShowResult{}, err
	}
	entry, ok := catalog[name]
	if !ok {
		return stackShowResult{}, machine.Wrap(machine.ErrorNotFound, fmt.Errorf("stack profile %q was not found", name), "List available Stack Profiles and select a configured name.", false)
	}
	resolved, err := development.ResolveStackProfile(name, development.ProfileMap(catalog))
	if err != nil {
		return stackShowResult{}, err
	}
	return stackShowResult{Scope: entry.Scope, Path: entry.Path, Sources: resolved.Sources, Profile: resolved.Profile}, nil
}
func createStackProfile(ctx context.Context, profile development.StackProfile, scope development.ProfileScope) (stackCreateResult, error) {
	if err := authorizeCurrentMCPContext(ctx, "stack.create", "", "", ""); err != nil {
		return stackCreateResult{}, err
	}
	if scope == "" {
		scope = development.ProfileScopeUser
	}
	if scope != development.ProfileScopeUser && scope != development.ProfileScopeRepository {
		return stackCreateResult{}, machine.Wrap(machine.ErrorValidationFailed, fmt.Errorf("unsupported stack scope"), "Choose user or repository.", false)
	}
	_, catalog, err := stackCatalog()
	if err != nil {
		return stackCreateResult{}, err
	}
	if _, exists := catalog[profile.Metadata.Name]; exists {
		return stackCreateResult{}, fmt.Errorf("stack profile %q already exists", profile.Metadata.Name)
	}
	temp := development.ProfileMap(catalog)
	temp[profile.Metadata.Name] = profile
	if _, err := development.ResolveStackProfile(profile.Metadata.Name, temp); err != nil {
		return stackCreateResult{}, err
	}
	path, err := development.SaveProfile(profile, scope, ".")
	if err != nil {
		return stackCreateResult{}, err
	}
	return stackCreateResult{Name: profile.Metadata.Name, Scope: scope, Path: path}, nil
}
