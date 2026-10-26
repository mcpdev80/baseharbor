package main

import (
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/development/goadapter"
	"github.com/mcpdev80/baseharbor/internal/development/nextjsadapter"
	"github.com/mcpdev80/baseharbor/internal/development/pythonadapter"
	"github.com/mcpdev80/baseharbor/internal/development/quarkusadapter"
)

func referenceDevelopmentRegistry() (development.Registry, error) {
	return development.NewRegistry(
		goadapter.Adapter{},
		nextjsadapter.Adapter{},
		pythonadapter.Adapter{},
		quarkusadapter.Adapter{},
	)
}

func builtinDevelopmentProfiles(registry development.Registry) map[string]development.StackProfile {
	profiles := map[string]development.StackProfile{}
	for _, id := range registry.IDs() {
		name := strings.TrimPrefix(id, "development/")
		profiles[name] = development.BuiltinProfile(id, name)
	}
	return profiles
}

func sortedProfileNames(entries development.ProfileCatalogEntries) []string {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		left, right := entries[names[i]], entries[names[j]]
		if left.Scope != right.Scope {
			order := map[development.ProfileScope]int{
				development.ProfileScopeBuiltin:    0,
				development.ProfileScopeUser:       1,
				development.ProfileScopeRepository: 2,
			}
			return order[left.Scope] < order[right.Scope]
		}
		return names[i] < names[j]
	})
	return names
}
