package application

import "fmt"

func describeMaterials(imp *ProviderImpact, r ProviderChangeRequest, fact ProviderImpactFact) (bool, bool, bool, error) {
	known, destructive, attention := fact.InventoryComplete, false, false
	requested := map[string]MaterialDisposition{}
	for _, d := range r.Materials {
		if _, duplicate := requested[d.MaterialID]; duplicate {
			return false, false, false, fmt.Errorf("duplicate material disposition")
		}
		if err := validateDisposition(d); err != nil {
			return false, false, false, err
		}
		requested[d.MaterialID] = d
	}
	for _, m := range fact.Materials {
		d, explicit := requested[m.ID]
		delete(requested, m.ID)
		if explicit && d.Disposition != "retained" && d.Disposition != "unknown" && m.Ownership != "baseharbor" {
			return false, false, false, &ProviderChangeError{Code: "material-ownership-unverified"}
		}
		meaning := "remains at the existing location; runtime retention does not prove continued application delivery"
		if !explicit {
			d = MaterialDisposition{MaterialID: m.ID, Disposition: "retained", Destination: m.Location, Supported: true, Verified: m.Verified, Evidence: m.Evidence}
			if !interruptsProvider(r.Operation) {
				d.AccessAfter = m.Access
			}
			if m.Location == nil {
				d.Disposition = "unknown"
				d.Supported = false
				d.Verified = false
				meaning = "retention/recoverability and destination are unknown; transient sessions, jobs and in-flight work are not assumed disposable"
			}
		}
		switch d.Disposition {
		case "deleted":
			destructive = true
			attention = true
			meaning = "no destination; deletion"
		case "transferred":
			imp.Prerequisites = append(imp.Prerequisites, ProviderPrerequisite{Kind: "material-handoff", Subject: m.ID, Supported: d.Supported, Verified: d.Verified, Evidence: d.Evidence, Consequence: "without verified handoff/access, old-provider removal has no verified material delivery"})
			if !d.Supported || !d.Verified || d.Destination == nil || d.Evidence == "" || (securityMaterial(m.Kind) && !hasSecureDeliveryReference(d.AccessAfter)) {
				known = false
				attention = true
				meaning = "supported secure handoff/access is not verified; this proposal is not a successful continuation"
			} else {
				meaning = "transfer to the specified existing destination; handoff and delivery verified before old-provider teardown"
			}
		case "revoked", "expired", "invalidated", "nonrecoverable":
			attention = true
			meaning = "material becomes " + d.Disposition + "; no working continuation is implied"
			if d.Disposition == "nonrecoverable" {
				destructive = true
			}
		case "unknown":
			known = false
			attention = true
		case "retained":
			if d.Destination == nil || m.Location == nil || *d.Destination != *m.Location {
				return false, false, false, fmt.Errorf("retained material must remain at its actual existing location")
			}
			if !m.Verified || !d.Verified {
				known = false
				attention = true
			}
		}
		if !m.Verified || m.Evidence == "" || m.Ownership == "" {
			known = false
		}
		imp.Materials = append(imp.Materials, MaterialConsequence{Before: m, After: d, Meaning: meaning})
	}
	if len(requested) > 0 {
		return false, false, false, &ProviderChangeError{Code: "material-inventory-missing"}
	}
	return known, destructive, attention, nil
}

func securityMaterial(kind string) bool {
	switch kind {
	case "secret", "lease", "key", "certificate", "credentials", "signing-key", "token", "recovery":
		return true
	}
	return false
}

func validateDisposition(d MaterialDisposition) error {
	if d.MaterialID == "" {
		return fmt.Errorf("material reference is required")
	}
	switch d.Disposition {
	case "retained", "transferred", "deleted", "revoked", "expired", "invalidated", "nonrecoverable", "unknown":
	default:
		return fmt.Errorf("invalid material disposition")
	}
	if (d.Disposition == "deleted" || d.Disposition == "nonrecoverable") && (d.Destination != nil || d.AccessAfter != nil) {
		return fmt.Errorf("deleted/nonrecoverable material cannot have a destination or access")
	}
	if d.Verified && d.Evidence == "" {
		return fmt.Errorf("verified material disposition requires evidence")
	}
	if d.Destination != nil {
		if err := validateLocation(*d.Destination); err != nil {
			return err
		}
	}
	return validateProviderAccess(d.AccessAfter)
}

func hasSecureDeliveryReference(a *ProviderAccess) bool {
	if a == nil {
		return false
	}
	return a.Security.Identity != nil || len(a.Security.Secrets) > 0 || len(a.Security.Credentials) > 0 || len(a.Security.Trust) > 0
}
