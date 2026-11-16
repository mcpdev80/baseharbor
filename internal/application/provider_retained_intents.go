package application

import (
	"fmt"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func validateRetainedConsumerIntents(consumer ConsumerIntent) error {
	for _, retained := range consumer.RetainedIntents {
		if !consumer.Active || consumer.Manifest.ApplicationID == "" || retained.ApplicationID != consumer.Manifest.ApplicationID || retained.Environment != consumer.Manifest.Environment {
			return fmt.Errorf("retained canonical intent requires the same active application/environment owner")
		}
		if err := retained.ValidateIntent(); err != nil {
			return err
		}
	}
	return nil
}

// Resolve every existing canonical projection through the SAME pure resolver.
// Pending intent protects the union of requirements and roots until its existing
// lifecycle owner verifies completion; it creates no additional consumer owner.
func resolveRetainedConsumerFootprint(snapshot ResolutionSnapshot, consumer ConsumerIntent) (Footprint, map[capability.Requirement]bool, error) {
	manifests := append([]Manifest{consumer.Manifest}, consumer.RetainedIntents...)
	wanted := map[capability.Requirement]bool{}
	for _, manifest := range manifests {
		reqs, err := requestedCapabilities(manifest)
		if err != nil {
			return Footprint{}, nil, err
		}
		for _, req := range reqs {
			wanted[req] = true
		}
	}
	reqs := []capability.Requirement{}
	for req := range wanted {
		reqs = append(reqs, req)
	}
	if _, _, err := resolvePreferences(reqs, consumer.Preferences); err != nil {
		return Footprint{}, nil, err
	}
	consumerSnapshot := snapshot
	consumerSnapshot.DependencyRoots = consumer.DependencyRoots
	var merged Footprint
	for index, manifest := range manifests {
		current, err := requestedCapabilities(manifest)
		if err != nil {
			return Footprint{}, nil, err
		}
		preferences := []ProviderPreference{}
		for _, preference := range consumer.Preferences {
			for _, req := range current {
				if preference.Requirement == req {
					preferences = append(preferences, preference)
				}
			}
		}
		fp, err := ResolveFootprint(manifest, consumerSnapshot, preferences)
		if err != nil {
			return Footprint{}, nil, err
		}
		if index == 0 {
			merged = fp
		} else {
			mergeRetainedFootprint(&merged, fp)
		}
	}
	return merged, wanted, nil
}

func mergeRetainedFootprint(merged *Footprint, retained Footprint) {
	merged.Complete = merged.Complete && retained.Complete
	if retained.Core == CoreRequired || (retained.Core == CoreUnknown && merged.Core != CoreRequired) {
		merged.Core = retained.Core
	}
	requirements := map[capability.Requirement]bool{}
	for _, req := range merged.Requirements {
		requirements[req.Requirement] = true
	}
	for _, req := range retained.Requirements {
		if !requirements[req.Requirement] {
			merged.Requirements = append(merged.Requirements, req)
			requirements[req.Requirement] = true
		}
	}
	providers := map[string]bool{}
	for _, group := range [][]ProviderFootprint{merged.Existing, merged.Additional} {
		for _, provider := range group {
			providers[provider.InstanceID] = true
		}
	}
	for _, group := range []struct {
		source      []ProviderFootprint
		destination *[]ProviderFootprint
	}{{retained.Existing, &merged.Existing}, {retained.Additional, &merged.Additional}} {
		for _, provider := range group.source {
			if !providers[provider.InstanceID] {
				*group.destination = append(*group.destination, provider)
				providers[provider.InstanceID] = true
			}
		}
	}
	unused := []string{}
	for _, id := range merged.Unused {
		if !providers[id] {
			unused = append(unused, id)
		}
	}
	merged.Unused = unused
	sort.Slice(merged.Requirements, func(i, j int) bool {
		a, b := merged.Requirements[i].Requirement, merged.Requirements[j].Requirement
		return string(a.Kind)+"/"+a.Name < string(b.Kind)+"/"+b.Name
	})
	sort.Slice(merged.Existing, func(i, j int) bool { return merged.Existing[i].InstanceID < merged.Existing[j].InstanceID })
	sort.Slice(merged.Additional, func(i, j int) bool { return merged.Additional[i].InstanceID < merged.Additional[j].InstanceID })
}
