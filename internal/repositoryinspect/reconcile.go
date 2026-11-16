package repositoryinspect

import "sort"

// Reconcile compares read-only repository evidence with the explicit application
// contract. It never mutates the manifest. In particular, "stale" means only
// that current inspection did not rediscover evidence; it is never permission
// to remove a declared capability.
func Reconcile(findings []Finding, declared []CapabilityIntent) ([]CapabilityIntent, []ReconciliationItem) {
	declared = append([]CapabilityIntent(nil), declared...)
	items := make([]ReconciliationItem, 0, len(declared)+len(findings))
	matchedFinding := make([]bool, len(findings))
	groups := evidenceBackedInstances(findings)
	claims := explicitInstanceClaims(declared)

	for _, intent := range declared {
		item := ReconciliationItem{
			Capability: intent.Capability,
			Name:       intent.Name,
			Direction:  intent.Direction,
			State:      ReconciliationStale,
		}
		best := -1
		for i, finding := range findings {
			if !findingMatchesIntentAliasesDefault(finding, intent, groups, claims) {
				continue
			}
			evidence := nonManifestEvidence(finding.Evidence)
			if len(evidence) == 0 {
				continue
			}
			matchedFinding[i] = true
			if best == -1 || confidenceRank(finding.Confidence) > confidenceRank(findings[best].Confidence) {
				best = i
			}
			item.Evidence = uniqueEvidence(append(item.Evidence, evidence...))
			item.Operations = uniqueRuntimeOperations(append(item.Operations, finding.Operations...))
		}
		if best >= 0 {
			item.State = ReconciliationSatisfied
			item.Confidence = findings[best].Confidence
		}
		items = append(items, item)
	}

	for i, finding := range findings {
		if matchedFinding[i] {
			continue
		}
		evidence := nonManifestEvidence(finding.Evidence)
		if len(evidence) == 0 {
			continue
		}
		state := ReconciliationAmbiguous
		if finding.Confidence == ConfidenceDetected {
			state = ReconciliationNew
		}
		items = append(items, ReconciliationItem{
			Capability: finding.Capability,
			Name:       finding.Name,
			Direction:  normalizedDirection(finding),
			State:      state,
			Operations: uniqueRuntimeOperations(finding.Operations),
			Confidence: finding.Confidence,
			Evidence:   evidence,
		})
	}

	sort.Slice(declared, func(i, j int) bool {
		if declared[i].Capability != declared[j].Capability {
			return declared[i].Capability < declared[j].Capability
		}
		return declared[i].Name < declared[j].Name
	})
	sort.Slice(items, func(i, j int) bool {
		if items[i].State != items[j].State {
			return items[i].State < items[j].State
		}
		if items[i].Capability != items[j].Capability {
			return items[i].Capability < items[j].Capability
		}
		return items[i].Name < items[j].Name
	})
	return declared, items
}

func findingMatchesIntent(finding Finding, intent CapabilityIntent) bool {
	if finding.Capability != intent.Capability {
		return false
	}
	direction := normalizedDirection(finding)
	if intent.Direction != "" && direction != "" && direction != intent.Direction {
		return false
	}
	if finding.Name == "" {
		return true
	}
	if finding.Capability == "telemetry.otlp" && intent.Name == "default" {
		switch finding.Name {
		case "traces", "metrics", "logs":
			return true
		}
	}
	return finding.Name == intent.Name
}

// instanceGroup identifies the distinct named instances detected for one
// capability in one direction.
type instanceGroup struct {
	capability string
	direction  Direction
}

// evidenceBackedInstances collects distinct named instance identities that
// carry non-manifest evidence. Duplicate evidence rows for the same instance
// name count once, so evidence volume never inflates ambiguity.
func evidenceBackedInstances(findings []Finding) map[instanceGroup]map[string]struct{} {
	groups := map[instanceGroup]map[string]struct{}{}
	for _, finding := range findings {
		if finding.Name == "" || len(nonManifestEvidence(finding.Evidence)) == 0 {
			continue
		}
		key := instanceGroup{finding.Capability, normalizedDirection(finding)}
		if groups[key] == nil {
			groups[key] = map[string]struct{}{}
		}
		groups[key][finding.Name] = struct{}{}
	}
	return groups
}

// instanceClaim is an explicit non-default identity declaration. Such claims
// are resolved before any canonical default alias may consider the same
// instance, independently of declaration order.
type instanceClaim struct {
	capability string
	name       string
	direction  Direction
}

func explicitInstanceClaims(declared []CapabilityIntent) []instanceClaim {
	claims := make([]instanceClaim, 0, len(declared))
	for _, intent := range declared {
		if intent.Name != "" && intent.Name != "default" {
			claims = append(claims, instanceClaim{intent.Capability, intent.Name, intent.Direction})
		}
	}
	return claims
}

// findingMatchesIntentAliasesDefault extends findingMatchesIntent with the
// fail-closed canonical default alias: a declared "default" instance may
// reconcile exactly one distinct evidence-backed named instance of the same
// capability and direction that is not claimed by an explicit declaration,
// keeping the declared name. More than one named instance is never guessed,
// and while instances are ambiguous, nameless evidence may not satisfy or
// conceal the declared default. telemetry.otlp keeps its dedicated signal
// rule unchanged.
func findingMatchesIntentAliasesDefault(finding Finding, intent CapabilityIntent, groups map[instanceGroup]map[string]struct{}, claims []instanceClaim) bool {
	if intent.Name != "default" || finding.Capability == "telemetry.otlp" {
		return findingMatchesIntent(finding, intent)
	}
	candidates := defaultInstanceCandidates(intent, finding, groups, claims)
	if finding.Name == "" {
		if len(candidates) > 1 {
			return false
		}
		return findingMatchesIntent(finding, intent)
	}
	if findingMatchesIntent(finding, intent) {
		return true
	}
	if finding.Capability != intent.Capability {
		return false
	}
	direction := normalizedDirection(finding)
	if intent.Direction != "" && direction != "" && direction != intent.Direction {
		return false
	}
	// The alias must bind the current finding itself, not any sole
	// candidate seen from another finding's perspective: an explicitly
	// claimed instance is excluded from candidates and must never be
	// consumed for the default declaration.
	return len(candidates) == 1 && candidates[0] == finding.Name
}

// defaultInstanceCandidates lists evidence-backed named instance identities
// a default intent could still alias in this direction: names explicitly
// claimed by another declaration are resolved first and excluded.
func defaultInstanceCandidates(intent CapabilityIntent, finding Finding, groups map[instanceGroup]map[string]struct{}, claims []instanceClaim) []string {
	direction := intent.Direction
	if direction == "" {
		direction = normalizedDirection(finding)
	}
	names := groups[instanceGroup{intent.Capability, direction}]
	candidates := make([]string, 0, len(names))
	for name := range names {
		if !nameExplicitlyClaimed(claims, intent.Capability, name, direction) {
			candidates = append(candidates, name)
		}
	}
	return candidates
}

func nameExplicitlyClaimed(claims []instanceClaim, capability, name string, direction Direction) bool {
	for _, claim := range claims {
		if claim.capability != capability || claim.name != name {
			continue
		}
		if claim.direction == "" || direction == "" || claim.direction == direction {
			return true
		}
	}
	return false
}

func normalizedDirection(finding Finding) Direction {
	if finding.Direction != "" {
		return finding.Direction
	}
	switch finding.Capability {
	case "metrics", "exposure.http":
		return DirectionProvide
	case "telemetry.otlp":
		return DirectionExport
	default:
		return DirectionConsume
	}
}

func nonManifestEvidence(evidence []Evidence) []Evidence {
	filtered := make([]Evidence, 0, len(evidence))
	for _, item := range evidence {
		if item.Kind != EvidenceManifest {
			filtered = append(filtered, item)
		}
	}
	return uniqueEvidence(filtered)
}

func uniqueRuntimeOperations(operations []RuntimeOperation) []RuntimeOperation {
	seen := map[RuntimeOperation]struct{}{}
	result := make([]RuntimeOperation, 0, len(operations))
	for _, operation := range operations {
		if operation == "" {
			continue
		}
		if _, ok := seen[operation]; ok {
			continue
		}
		seen[operation] = struct{}{}
		result = append(result, operation)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
