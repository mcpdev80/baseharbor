package application

func functionOutcomesMatch(actual []ProviderFunctionOutcome, planned []FunctionConsequence) bool {
	byKey := map[string]ProviderFunctionOutcome{}
	for _, f := range actual {
		key := f.Consumer + "/" + f.InstanceID + "/" + f.Function
		if _, duplicate := byKey[key]; duplicate {
			return false
		}
		byKey[key] = f
	}
	if len(byKey) != len(planned) {
		return false
	}
	for _, expected := range planned {
		if len(expected.Via) == 0 {
			return false
		}
		got, exists := byKey[expected.Consumer+"/"+expected.Via[0]+"/"+expected.Function]
		if !exists || !got.Verified || got.Evidence == "" || got.Outcome == "unknown" || got.Outcome != expected.Outcome || got.ValidUntil != expected.ValidUntil || got.Behavior != expected.Behavior || !sameConsumerTransition(expected.Transition, got.Transition) {
			return false
		}
	}
	return true
}

func effectOutcomesMatch(actual, expected []OperationalConsequence) bool {
	if len(actual) != len(expected) {
		return false
	}
	seen := map[int]bool{}
	for _, e := range expected {
		found := false
		for i, a := range actual {
			if !seen[i] && a.Aspect == e.Aspect && a.Before == e.Before && a.After == e.After && a.Evidence != "" {
				seen[i] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
