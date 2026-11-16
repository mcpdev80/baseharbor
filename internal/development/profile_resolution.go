package development

import (
	"fmt"
	"sort"
	"strings"
)

type ProfileCatalog map[string]StackProfile

type ProfileConflict struct {
	Field    string `json:"field"`
	Current  string `json:"current"`
	Incoming string `json:"incoming"`
}

type ProfileResolution struct {
	Profile   StackProfile      `json:"profile"`
	Sources   []string          `json:"sources"`
	Conflicts []ProfileConflict `json:"conflicts,omitempty"`
}

func ResolveStackProfile(name string, catalog ProfileCatalog) (ProfileResolution, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ProfileResolution{}, fmt.Errorf("stack profile name is required")
	}
	visiting := map[string]bool{}
	resolved := map[string]StackProfile{}
	var order []string

	var resolve func(string) (StackProfile, error)
	resolve = func(current string) (StackProfile, error) {
		if profile, ok := resolved[current]; ok {
			return profile, nil
		}
		if visiting[current] {
			return StackProfile{}, fmt.Errorf("stack profile composition cycle at %q", current)
		}
		profile, ok := catalog[current]
		if !ok {
			return StackProfile{}, fmt.Errorf("stack profile %q was not found", current)
		}
		visiting[current] = true

		base := StackProfile{
			APIVersion: StackProfileAPIVersion,
			Kind:       StackProfileKind,
			Metadata:   ProfileMetadata{Name: profile.Metadata.Name},
		}
		for _, parentName := range profile.Extends {
			parentName = strings.TrimSpace(parentName)
			parent, err := resolve(parentName)
			if err != nil {
				return StackProfile{}, err
			}
			var mergeErr error
			base, mergeErr = mergeProfiles(base, parent, false)
			if mergeErr != nil {
				return StackProfile{}, fmt.Errorf("compose stack profile %q from %q: %w", current, parentName, mergeErr)
			}
		}
		var err error
		base, err = mergeProfiles(base, profile, true)
		if err != nil {
			return StackProfile{}, fmt.Errorf("compose stack profile %q: %w", current, err)
		}
		base.Extends = nil
		if err := base.Validate(); err != nil {
			return StackProfile{}, err
		}
		visiting[current] = false
		resolved[current] = base
		order = append(order, current)
		return base, nil
	}

	profile, err := resolve(name)
	if err != nil {
		return ProfileResolution{}, err
	}
	return ProfileResolution{Profile: profile, Sources: uniqueProfileSources(order)}, nil
}

func mergeProfiles(base, incoming StackProfile, allowOverride bool) (StackProfile, error) {
	if incoming.APIVersion != "" && incoming.APIVersion != StackProfileAPIVersion {
		return StackProfile{}, fmt.Errorf("unsupported apiVersion %q", incoming.APIVersion)
	}
	if incoming.Kind != "" && incoming.Kind != StackProfileKind {
		return StackProfile{}, fmt.Errorf("unsupported kind %q", incoming.Kind)
	}
	if incoming.Metadata.Name != "" {
		base.Metadata.Name = incoming.Metadata.Name
	}

	components := map[string]Component{}
	for _, component := range base.Components {
		components[component.ID] = component
	}
	for _, component := range incoming.Components {
		if current, exists := components[component.ID]; exists && current != component && !allowOverride {
			return StackProfile{}, fmt.Errorf("component %q conflicts between parent profiles", component.ID)
		}
		components[component.ID] = component
	}
	base.Components = base.Components[:0]
	for _, component := range components {
		base.Components = append(base.Components, component)
	}
	sort.Slice(base.Components, func(i, j int) bool { return base.Components[i].ID < base.Components[j].ID })

	preferences := map[string]CapabilityPreference{}
	for _, preference := range base.Capabilities {
		preferences[profilePreferenceKey(preference)] = preference
	}
	for _, preference := range incoming.Capabilities {
		key := profilePreferenceKey(preference)
		if current, exists := preferences[key]; exists && !sameCapabilityPreference(current, preference) && !allowOverride {
			return StackProfile{}, fmt.Errorf("capability preference %q conflicts between parent profiles", key)
		}
		preferences[key] = preference
	}
	base.Capabilities = base.Capabilities[:0]
	for _, preference := range preferences {
		copyPreference := preference
		copyPreference.Components = append([]string(nil), preference.Components...)
		sort.Strings(copyPreference.Components)
		base.Capabilities = append(base.Capabilities, copyPreference)
	}
	sort.Slice(base.Capabilities, func(i, j int) bool {
		return profilePreferenceKey(base.Capabilities[i]) < profilePreferenceKey(base.Capabilities[j])
	})
	return base, nil
}

func profilePreferenceKey(preference CapabilityPreference) string {
	components := append([]string(nil), preference.Components...)
	sort.Strings(components)
	return string(preference.Capability) + ":" + strings.Join(components, ",")
}

func sameCapabilityPreference(a, b CapabilityPreference) bool {
	return profilePreferenceKey(a) == profilePreferenceKey(b) &&
		a.ImplementationPreference == b.ImplementationPreference &&
		a.DevelopmentIntegration == b.DevelopmentIntegration
}

func uniqueProfileSources(values []string) []string {
	seen := map[string]struct{}{}
	var result []string
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
