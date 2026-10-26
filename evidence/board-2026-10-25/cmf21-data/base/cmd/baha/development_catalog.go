package main

import (
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/orgconfig"
)

func effectiveDevelopmentProfileCatalog(repositoryRoot string, registry development.Registry) (development.ProfileCatalogEntries, error) {
	organization, err := organizationStackProfilePaths()
	if err != nil {
		return nil, err
	}
	return development.LoadProfileCatalogWithOrganization(repositoryRoot, builtinDevelopmentProfiles(registry), organization)
}

func organizationStackProfilePaths() (map[string]string, error) {
	state, ok, err := orgconfig.LoadActiveOptional()
	if err != nil || !ok {
		return nil, err
	}
	result := map[string]string{}
	for name, ref := range state.Config.Stacks {
		path, cached, err := orgconfig.ResolveCachedArtifact(state, ref.Reference)
		if err != nil {
			return nil, fmt.Errorf("resolve organization stack %q: %w", name, err)
		}
		if !cached {
			continue
		}
		result[name] = path
	}
	return result, nil
}

func organizationDefaultStack(environment string) (string, error) {
	state, ok, err := orgconfig.LoadActiveOptional()
	if err != nil || !ok {
		return "", err
	}
	effective, err := orgconfig.ResolveEffective(state, environment)
	if err != nil {
		return "", fmt.Errorf("resolve organization stack default: %w", err)
	}
	if effective.Stack == nil {
		return "", nil
	}
	return strings.TrimSpace(effective.Stack.Name), nil
}
