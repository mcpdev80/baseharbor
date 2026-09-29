package development

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

type ProfileCatalog map[string]StackProfile

func (c ProfileCatalog) Resolve(name string) (StackProfile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return StackProfile{}, fmt.Errorf("stack profile name is required")
	}
	resolving := map[string]bool{}
	resolved := map[string]StackProfile{}

	var resolve func(string) (StackProfile, error)
	resolve = func(current string) (StackProfile, error) {
		if profile, ok := resolved[current]; ok {
			return profile, nil
		}
		if resolving[current] {
			return StackProfile{}, fmt.Errorf("stack profile composition cycle includes %q", current)
		}
		profile, ok := c[current]
		if !ok {
			return StackProfile{}, fmt.Errorf("stack profile %q is not available", current)
		}
		if err := profile.Validate(); err != nil {
			return StackProfile{}, err
		}
		resolving[current] = true
		effective := StackProfile{
			APIVersion: StackProfileAPIVersion,
			Kind:       StackProfileKind,
			Metadata:   ProfileMetadata{Name: profile.Metadata.Name},
		}
		for _, parentName := range profile.Extends {
			parent, err := resolve(strings.TrimSpace(parentName))
			if err != nil {
				return StackProfile{}, err
			}
			effective, err = mergeProfiles(effective, parent)
			if err != nil {
				return StackProfile{}, fmt.Errorf("compose stack profile %q from %q: %w", current, parentName, err)
			}
		}
		var err error
		effective, err = mergeProfiles(effective, profile)
		if err != nil {
			return StackProfile{}, fmt.Errorf("compose stack profile %q: %w", current, err)
		}
		effective.Metadata.Name = profile.Metadata.Name
		effective.Extends = append([]string(nil), profile.Extends...)
		delete(resolving, current)
		resolved[current] = effective
		return effective, nil
	}

	return resolve(name)
}

func mergeProfiles(base, overlay StackProfile) (StackProfile, error) {
	result := base
	if result.APIVersion == "" {
		result.APIVersion = StackProfileAPIVersion
	}
	if result.Kind == "" {
		result.Kind = StackProfileKind
	}

	components := map[string]Component{}
	for _, component := range result.Components {
		components[component.ID] = component
	}
	for _, component := range overlay.Components {
		if existing, ok := components[component.ID]; ok {
			if existing != component {
				return StackProfile{}, fmt.Errorf("component %q conflicts between composed profiles", component.ID)
			}
			continue
		}
		components[component.ID] = component
	}
	result.Components = result.Components[:0]
	componentIDs := make([]string, 0, len(components))
	for id := range components {
		componentIDs = append(componentIDs, id)
	}
	sort.Strings(componentIDs)
	for _, id := range componentIDs {
		result.Components = append(result.Components, components[id])
	}

	preferences := map[string]CapabilityPreference{}
	addPreference := func(preference CapabilityPreference) error {
		key := capabilityPreferenceKey(preference)
		if existing, ok := preferences[key]; ok {
			if !reflect.DeepEqual(normalizedPreference(existing), normalizedPreference(preference)) {
				return fmt.Errorf("capability preference %q conflicts between composed profiles", key)
			}
			return nil
		}
		preferences[key] = preference
		return nil
	}
	for _, preference := range result.Capabilities {
		if err := addPreference(preference); err != nil {
			return StackProfile{}, err
		}
	}
	for _, preference := range overlay.Capabilities {
		if err := addPreference(preference); err != nil {
			return StackProfile{}, err
		}
	}
	result.Capabilities = result.Capabilities[:0]
	keys := make([]string, 0, len(preferences))
	for key := range preferences {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result.Capabilities = append(result.Capabilities, preferences[key])
	}
	return result, nil
}

func capabilityPreferenceKey(preference CapabilityPreference) string {
	components := append([]string(nil), preference.Components...)
	sort.Strings(components)
	return string(preference.Capability) + ":" + strings.Join(components, ",")
}

func normalizedPreference(preference CapabilityPreference) CapabilityPreference {
	preference.Components = append([]string(nil), preference.Components...)
	sort.Strings(preference.Components)
	return preference
}
