package application

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/provider/builtin"
)

// WithProviderPreferences is a detached, non-serialized deployment projection.
// A non-nil empty selection explicitly resets to automatic; nil inherits state.
func WithProviderPreferences(m Manifest, preferences []ProviderPreference) Manifest {
	copy := append([]ProviderPreference{}, preferences...)
	m.providerPreferences = &copy
	return m
}

// DesiredProviderPreferencesAt reads intent from the canonical deployment owner,
// never from portable application source or another Target's registry.
func DesiredProviderPreferencesAt(root string, m Manifest) ([]ProviderPreference, error) {
	if m.providerPreferences != nil {
		return append([]ProviderPreference{}, (*m.providerPreferences)...), nil
	}
	target := filepath.Base(filepath.Clean(root))
	expected, err := deployment.TargetStateRoot(target)
	if err != nil {
		return nil, nil
	}
	actual, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	canonical, err := filepath.Abs(expected)
	if err != nil {
		return nil, err
	}
	if actual != canonical {
		return nil, nil
	}
	record, found, err := deployment.FindDeployment(target, m.ApplicationID, m.Environment)
	if err != nil || !found {
		return nil, err
	}
	if len(record.DesiredProviderPreferences) == 0 {
		return nil, nil
	}
	var prefs []ProviderPreference
	decoder := json.NewDecoder(bytes.NewReader(record.DesiredProviderPreferences))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&prefs); err != nil {
		return nil, fmt.Errorf("invalid desired provider preferences: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid desired provider preferences trailing data")
	}
	return prefs, nil
}

// ValidateProviderPreferencesAt proves full requirement/instance identity against
// the current Target registry. It does not publish bindings or runtime facts.
func ValidateProviderPreferencesAt(root string, m Manifest, prefs []ProviderPreference) error {
	store, err := referenceProviderRegistryStoreAt(root)
	if err != nil {
		return err
	}
	registry, err := store.Load()
	if err != nil {
		return err
	}
	// Only canonical, locally owned reference instances may be selected before
	// their first apply. Arbitrary missing IDs remain rejected.
	contract, err := PortableContractFromManifest(m)
	if err != nil {
		return err
	}
	resources, err := ResolveCapabilityResources(contract)
	if err != nil {
		return err
	}
	for _, resource := range resources {
		instance, e := referenceProviderInstance(m, resource)
		if e != nil {
			continue
		}
		present := false
		for _, existing := range registry.Instances {
			if existing.ID == instance.ID {
				present = true
			}
		}
		if !present && instance.Ownership == capability.OwnershipBaseHarbor {
			// Do not register here: read-only validation must not publish it.
			registry.Instances = append(registry.Instances, instance)
		}
	}
	reqs, err := requestedCapabilities(m)
	if err != nil {
		return err
	}
	if _, _, err := resolvePreferences(reqs, prefs); err != nil {
		return err
	}
	for _, pref := range prefs {
		if pref.InstanceID == "" && pref.ProviderKind == "" {
			continue
		}
		found := false
		for _, instance := range registry.Instances {
			if pref.InstanceID != "" && instance.ID != pref.InstanceID {
				continue
			}
			if pref.ProviderKind != "" && instance.Provider.Kind != pref.ProviderKind {
				continue
			}
			if !instance.Provider.Supports(pref.Requirement.Kind) || !eligibleInstance(instance, m) {
				continue
			}
			found = true
		}
		if !found {
			return fmt.Errorf("selected provider for %s/%s is missing or incompatible", pref.Requirement.Kind, pref.Requirement.Name)
		}
	}
	return nil
}

// SaveDesiredProviderPreferencesAt must be called by the authorized caller under
// its existing Target lifecycle lock, after consent and deployment registration.
func SaveDesiredProviderPreferencesAt(root, target string, m Manifest, prefs []ProviderPreference) error {
	expected, err := deployment.TargetStateRoot(target)
	if err != nil {
		return err
	}
	actual, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	canonical, err := filepath.Abs(expected)
	if err != nil || actual != canonical {
		return errors.New("provider preference Target root differs")
	}
	if err := ValidateProviderPreferencesAt(root, m, prefs); err != nil {
		return err
	}
	record, found, err := deployment.FindDeployment(target, m.ApplicationID, m.Environment)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("provider preference deployment owner is missing")
	}
	prefs = append([]ProviderPreference{}, prefs...)
	registry, err := PreviewReferenceProviderRegistryAt(root, WithProviderPreferences(m, prefs))
	if err != nil {
		return err
	}
	for i := range prefs {
		for _, instance := range registry.Instances {
			if instance.ID == prefs[i].InstanceID {
				prefs[i].ProviderKind = instance.Provider.Kind
			}
		}
	}
	raw, err := json.Marshal(append([]ProviderPreference{}, prefs...))
	if err != nil {
		return err
	}
	record.DesiredProviderPreferences = raw
	return deployment.SaveDeploymentRecord(record)
}

func preferredProviderResource(registry *capability.Registry, m Manifest, resource capability.Resource) (bool, error) {
	if m.providerPreferences == nil {
		return false, nil
	}
	for _, pref := range *m.providerPreferences {
		if pref.Requirement != (capability.Requirement{Kind: resource.Kind, Name: resource.Name}) {
			continue
		}
		if pref.InstanceID == "" && pref.ProviderKind == "" {
			return false, nil
		}
		present := false
		for _, instance := range registry.Instances {
			if instance.ID == pref.InstanceID {
				present = true
			}
		}
		if !present && pref.InstanceID != "" {
			candidate, err := referenceProviderInstance(m, resource)
			if err == nil && candidate.ID == pref.InstanceID && candidate.Ownership == capability.OwnershipBaseHarbor {
				if err := registry.Register(candidate); err != nil {
					return true, err
				}
			}
		}
		selected := ""
		for _, instance := range registry.Instances {
			if pref.InstanceID != "" && instance.ID != pref.InstanceID {
				continue
			}
			if pref.ProviderKind != "" && instance.Provider.Kind != pref.ProviderKind {
				continue
			}
			if !instance.Provider.Supports(resource.Kind) || !eligibleInstance(instance, m) {
				continue
			}
			if selected != "" {
				return true, errors.New("ambiguous selected provider kind")
			}
			selected = instance.ID
			resource.Provider = instance.Provider.Kind
		}
		if selected == "" {
			return true, errors.New("selected provider instance missing or incompatible")
		}
		return true, registry.BindDeployment(resource, m.ApplicationID, m.Environment, selected)
	}
	return false, nil
}

// WithDesiredProviderPlacementsAt connects exact desired selections to the
// existing native placement owner, without changing retained/applied facts.
func WithDesiredProviderPlacementsAt(root string, m Manifest) (Manifest, error) {
	prefs, err := DesiredProviderPreferencesAt(root, m)
	if err != nil {
		return Manifest{}, err
	}
	if err := ValidateProviderPreferencesAt(root, m, prefs); err != nil {
		return Manifest{}, err
	}
	store, err := referenceProviderRegistryStoreAt(root)
	if err != nil {
		return Manifest{}, err
	}
	registry, err := store.Load()
	if err != nil {
		return Manifest{}, err
	}
	selected := map[capability.ProviderKind]capability.ProviderPlacement{}
	for _, pref := range prefs {
		if pref.InstanceID == "" {
			continue
		}
		for _, instance := range registry.Instances {
			if instance.ID != pref.InstanceID {
				continue
			}
			placement := capability.ProviderPlacement{Scope: instance.Scope, SharingBoundary: instance.SharingBoundary, Ownership: instance.Ownership, ExternalReference: instance.Reference}
			provider, err := builtin.Lookup(instance.Provider.Kind)
			if err != nil {
				return Manifest{}, err
			}
			if err := capability.ValidateProviderPlacement(provider.Integration, placement); err != nil {
				return Manifest{}, err
			}
			if instance.Ownership == capability.OwnershipBaseHarbor {
				owner := m
				owner.backendPlacements = map[capability.ProviderKind]capability.ProviderPlacement{instance.Provider.Kind: placement}
				expected, err := referenceProviderInstance(owner, capability.Resource{Application: m.Name, Kind: pref.Requirement.Kind, Name: pref.Requirement.Name, Provider: instance.Provider.Kind})
				if err != nil {
					return Manifest{}, err
				}
				if expected.ID != instance.ID {
					return Manifest{}, errors.New("selected instance has no matching canonical native execution owner")
				}
			}
			if old, ok := selected[instance.Provider.Kind]; ok && old != placement {
				return Manifest{}, errors.New("native provider owner cannot realize conflicting requirement placements")
			}
			selected[instance.Provider.Kind] = placement
		}
	}
	placements := map[capability.ProviderKind]capability.ProviderPlacement{}
	for kind, placement := range m.backendPlacements {
		placements[kind] = placement
	}
	for kind, placement := range selected {
		placements[kind] = placement
	}
	m.backendPlacements = placements
	return m, nil
}
