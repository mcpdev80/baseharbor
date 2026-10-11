package application

// Recovery follows the same aggregate material plan, including backups owned by
// another changed provider. Unselected/foreign recovery remains preserved. An
// explicit authorized backup deletion must not be mistaken for accidental loss.
func expectedMaterialRecovery(source MaterialConsequence, aggregate []ProviderImpact) []ResourceLocation {
	expected := []ResourceLocation{}
	for _, location := range source.Before.Recovery {
		after := location
		deleted := false
		for _, impact := range aggregate {
			for _, material := range impact.Materials {
				if material.Before.Ownership != "baseharbor" || material.Before.Location == nil || *material.Before.Location != location {
					continue
				}
				switch material.After.Disposition {
				case "deleted", "nonrecoverable":
					deleted = true
				case "transferred":
					if material.After.Destination != nil {
						after = *material.After.Destination
					}
				}
			}
		}
		if !deleted {
			expected = append(expected, after)
		}
	}
	return expected
}
