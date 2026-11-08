package application

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
		if present || instance.Ownership != capability.OwnershipBaseHarbor {
			continue
		}
		// Do not register here: read-only validation must not publish it.
		registry.Instances = append(registry.Instances, instance)
		// The projection may be stale for an instance another (even unapplied)
		// record still registers. Re-adopt the authoritative live record when
		// it exists, so a live shared instance is validated by its real
		// ownership shape instead of an invented projection.
		if live, err := liveProviderRegistryAt(root); err == nil {
			for _, liveInstance := range live.Instances {
				if liveInstance.ID == instance.ID && liveInstance.Provider.Kind == instance.Provider.Kind {
					registry.Instances[len(registry.Instances)-1] = liveInstance
					break
				}
			}
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
		if !found && pref.InstanceID != "" {
			// Registry drift: the instance lives on the Target (the live
			// registry is the provider's own inventory) even though this
			// projection lost it. Validate it by its live identity.
			if resolvable, _ := PreferenceResolvableViaLive(pref); resolvable {
				found = true
			}
		}
		if !found {
			if pref.InstanceID != "" {
				return providerPreferenceUnresolvedError(pref)
			}
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
		if prefs[i].ProviderKind != "" || prefs[i].InstanceID == "" {
			continue
		}
		// An explicitly selected canonical instance that the preview never
		// registered (absent from the stale Target projection) still has one
		// bundled provider kind; persisting the choice without it would
		// silently downgrade it to automatic on the next read.
		contract, err := PortableContractFromManifest(m)
		if err != nil {
			return err
		}
		resources, err := ResolveCapabilityResources(contract)
		if err != nil {
			return err
		}
		pinned := false
		for _, resource := range resources {
			if resource.Kind != prefs[i].Requirement.Kind || resource.Name != prefs[i].Requirement.Name {
				continue
			}
			if candidate, err := referenceProviderInstance(m, resource); err == nil && candidate.ID == prefs[i].InstanceID {
				prefs[i].ProviderKind = candidate.Provider.Kind
				pinned = true
			}
		}
		if !pinned && prefs[i].ProviderKind == "" {
			return fmt.Errorf("provider preference %s/%s references unknown instance %q", prefs[i].Requirement.Kind, prefs[i].Requirement.Name, prefs[i].InstanceID)
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
	return preferredProviderResourceAt("", registry, m, resource)
}

// preferredProviderResourceAt resolves one explicit provider preference against
// the projected registry first and, only when the selected instance is absent
// there, against the live provider registry stored at root. A stale projection
// must never block the consumer: an instance that is live and remains the
// canonical native realization of this resource is re-adopted with its live
// scope and ownership record. An instance that is gone, or live but no longer
// the canonical realization, fails closed with an actionable error.
func preferredProviderResourceAt(root string, registry *capability.Registry, m Manifest, resource capability.Resource) (bool, error) {
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
			candidate, candidateErr := referenceProviderInstance(m, resource)
			if candidateErr == nil && candidate.ID == pref.InstanceID && candidate.Ownership == capability.OwnershipBaseHarbor {
				// Canonical but not yet applied: project the known realization.
				if err := registry.Register(candidate); err != nil {
					return true, err
				}
			} else if liveErr := readoptLiveProviderInstance(root, registry, resource, pref.InstanceID); liveErr != nil {
				// The projection lost this instance. Re-adopt it only when the
				// live Target registry still registers that exact identity; a
				// genuinely gone instance fails closed with its next step.
				return true, liveErr
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
			return true, providerPreferenceUnresolvedError(pref)
		}
		return true, registry.BindDeployment(resource, m.ApplicationID, m.Environment, selected)
	}
	return false, nil
}

func providerPreferenceUnresolvedError(pref ProviderPreference) error {
	if pref.InstanceID != "" {
		return fmt.Errorf("provider instance %q for %s/%s is no longer registered on this Target; select the requirement with 'provider %s/%s automatic' to re-resolve it against the current provider inventory", pref.InstanceID, pref.Requirement.Kind, pref.Requirement.Name, pref.Requirement.Kind, pref.Requirement.Name)
	}
	return errors.New("selected provider instance missing or incompatible")
}

// PreferenceResolvableViaLive reports whether an explicit provider preference
// can still be resolved by the plan authority: the live provider registry
// registers the exact instance under a BaseHarbor-owned provider that supports
// the requirement. It publishes nothing.
func PreferenceResolvableViaLive(pref ProviderPreference) (bool, error) {
	if pref.InstanceID == "" {
		return false, nil
	}
	dataRoot, err := deployment.DataRoot()
	if err != nil {
		return false, err
	}
	live, err := liveProviderRegistryAt(dataRoot)
	if err != nil {
		return false, nil
	}
	for _, instance := range live.Instances {
		if instance.ID != pref.InstanceID || instance.Ownership != capability.OwnershipBaseHarbor || !instance.Provider.Supports(pref.Requirement.Kind) {
			continue
		}
		if pref.ProviderKind != "" && instance.Provider.Kind != pref.ProviderKind {
			continue
		}
		return true, nil
	}
	return false, nil
}

// readoptLiveProviderInstance consults the authoritative live registry for an
// explicitly selected instance that the projection lost. The live Target
// registry is the provider's own inventory: the instance that is still there,
// is BaseHarbor-owned and supports the capability is re-adopted with its live
// ownership record. A genuinely removed instance, or one that no longer
// supports this capability, fails closed with an actionable error.
func readoptLiveProviderInstance(root string, registry *capability.Registry, resource capability.Resource, selectedID string) error {
	live, err := liveProviderRegistryAt(root)
	if err != nil {
		return providerPreferenceUnresolvedError(ProviderPreference{
			Requirement: capability.Requirement{Kind: resource.Kind, Name: resource.Name}, InstanceID: selectedID,
		})
	}
	for _, instance := range live.Instances {
		if instance.ID != selectedID {
			continue
		}
		if instance.Ownership != capability.OwnershipBaseHarbor || !instance.Provider.Supports(resource.Kind) {
			return fmt.Errorf("provider instance %q is not an eligible BaseHarbor provider for %s/%s on this Target; re-resolve the requirement with automatic selection", selectedID, resource.Kind, resource.Name)
		}
		if err := registry.Register(instance); err != nil {
			return err
		}
		return nil
	}
	return providerPreferenceUnresolvedError(ProviderPreference{
		Requirement: capability.Requirement{Kind: resource.Kind, Name: resource.Name}, InstanceID: selectedID,
	})
}

func liveProviderRegistryAt(root string) (capability.Registry, error) {
	if strings.TrimSpace(root) == "" {
		return capability.Registry{}, os.ErrNotExist
	}
	store, err := referenceProviderRegistryStoreAt(root)
	if err != nil {
		return capability.Registry{}, err
	}
	return store.Load()
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
