package application

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func validateLocation(l ResourceLocation) error {
	if l.Owner == "" || l.Target == "" || l.Reference == "" {
		return fmt.Errorf("location requires actual owner, Target and reference")
	}
	switch l.Kind {
	case "volume", "path", "external-store", "backup", "recovery":
	default:
		return fmt.Errorf("unsupported resource location kind")
	}
	return safeImpactText(l.Reference)
}
func validateProviderAccess(a *ProviderAccess) error {
	if a == nil {
		return nil
	}
	if a.Method == "" {
		return fmt.Errorf("access method is required")
	}
	if err := a.Security.Validate(); err != nil {
		return fmt.Errorf("invalid impact security references")
	}
	if a.Endpoint != "" {
		u, err := url.Parse(a.Endpoint)
		if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("impact endpoint must not contain credentials, query or fragment")
		}
		if err := safeImpactText(a.Endpoint); err != nil {
			return err
		}
	}
	return nil
}
func safeImpactText(value string) error {
	lower := strings.ToLower(value)
	if strings.ContainsAny(value, "\r\n\x00") || strings.Contains(lower, "private key") || strings.Contains(lower, "password=") || strings.Contains(lower, "token=") || strings.Contains(lower, "secret_id=") {
		return fmt.Errorf("impact metadata must contain authorized references only")
	}
	if strings.Contains(value, "://") {
		// URLs inside explanatory text are not a supported channel for secrets.
		for _, word := range strings.Fields(value) {
			if !strings.Contains(word, "://") {
				continue
			}
			u, err := url.Parse(word)
			if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return fmt.Errorf("impact metadata contains an unsafe reference")
			}
		}
	}
	return nil
}
func validateExpiry(value string) error {
	if value == "" {
		return nil
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("impact validity must be RFC3339")
	}
	return nil
}
func validateImpactObservation(o ProviderImpactObservation) error {
	// Validate every string, including error/evidence descriptions, without ever
	// echoing rejected content into logs or machine errors.
	raw, err := json.Marshal(o)
	if err != nil {
		return err
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return err
	}
	if err := validateImpactStrings(tree); err != nil {
		return err
	}
	for id, f := range o.Providers {
		if f.Realization.InstanceID != id {
			return fmt.Errorf("observation instance key mismatch")
		}
		if f.Realization.Verified && f.Realization.Evidence == "" {
			return fmt.Errorf("verified realization requires evidence")
		}
		for _, l := range f.Realization.Locations {
			if err := validateLocation(l); err != nil {
				return err
			}
		}
		if err := validateProviderAccess(f.Realization.Access); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, m := range f.Materials {
			if m.ID == "" || m.Kind == "" || seen[m.ID] {
				return fmt.Errorf("missing/duplicate material reference")
			}
			seen[m.ID] = true
			if m.Verified && m.Evidence == "" {
				return fmt.Errorf("verified material requires evidence")
			}
			if m.Location != nil {
				if err := validateLocation(*m.Location); err != nil {
					return err
				}
			}
			for _, l := range m.Recovery {
				if err := validateLocation(l); err != nil {
					return err
				}
			}
			if err := validateProviderAccess(m.Access); err != nil {
				return err
			}
			if err := validateExpiry(m.ExpiresAt); err != nil {
				return err
			}
		}
		for _, e := range f.Effects {
			if e.Aspect == "" || e.Before == "" || e.After == "" {
				return fmt.Errorf("operational effect requires aspect and before/after")
			}
		}
	}
	for _, c := range o.Continuations {
		if c.Consumer == "" || c.Function == "" || c.From == "" || c.To == "" || (c.Verified && (!c.Supported || c.Evidence == "")) {
			return fmt.Errorf("invalid consumer continuation verification")
		}
	}
	for _, use := range o.Providers {
		for _, p := range use.Prerequisites {
			if p.Kind == "" || p.Subject == "" || p.Consequence == "" || (p.Verified && p.Evidence == "") {
				return fmt.Errorf("invalid prerequisite verification")
			}
		}
	}
	for _, use := range o.Functions {
		if use.Consumer == "" || use.InstanceID == "" || use.Function == "" {
			return fmt.Errorf("function use requires consumer, instance and function")
		}
		if err := validateExpiry(use.ValidUntil); err != nil {
			return err
		}
	}
	return nil
}
func validateImpactStrings(tree any) error {
	switch v := tree.(type) {
	case string:
		return safeImpactText(v)
	case []any:
		for _, entry := range v {
			if err := validateImpactStrings(entry); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, entry := range v {
			if err := safeImpactText(key); err != nil {
				return err
			}
			if err := validateImpactStrings(entry); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRequestStrings(r ProviderChangeRequest) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return err
	}
	return validateImpactStrings(tree)
}

func validateObservationOwners(s ResolutionSnapshot, o ProviderImpactObservation) error {
	instances, err := indexResolutionInstances(s)
	if err != nil {
		return err
	}
	for _, i := range instances {
		f, ok := o.Providers[i.ID]
		if !ok {
			continue
		}
		r := f.Realization
		if r.Ancillary != s.Facts[i.ID].Ancillary || r.Kind != i.Provider.Kind || r.Scope != i.Scope || r.Ownership != i.Ownership || ((i.Scope == "application" || s.Facts[i.ID].Ancillary) && r.Owner != observationOwner(i, s)) || r.Target == "" {
			return &ProviderChangeError{Code: "provider-observation-ownership-mismatch"}
		}
		if observed, ok := s.Facts[i.ID]; ok && r.Verified && (r.State != observed.State || r.Target != observed.Target) {
			return &ProviderChangeError{Code: "provider-observation-state-mismatch"}
		}
	}
	return nil
}
