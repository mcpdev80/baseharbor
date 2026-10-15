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

	for _, intent := range declared {
		item := ReconciliationItem{
			Capability: intent.Capability,
			Name:       intent.Name,
			Direction:  intent.Direction,
			State:      ReconciliationStale,
		}
		best := -1
		for i, finding := range findings {
			if !findingMatchesIntentAliasesDefault(finding, intent, groups) {
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

// findingMatchesIntentAliasesDefault extends findingMatchesIntent with the
// fail-closed canonical default alias: a declared "default" instance may
// reconcile exactly one distinct evidence-backed named instance of the same
// capability and direction, keeping the declared name. More than one named
// instance is never guessed, and while instances are ambiguous, nameless
// evidence may not satisfy or conceal the declared default. telemetry.otlp
// keeps its dedicated signal rule unchanged.
func findingMatchesIntentAliasesDefault(finding Finding, intent CapabilityIntent, groups map[instanceGroup]map[string]struct{}) bool {
	if intent.Name != "default" || finding.Capability == "telemetry.otlp" {
		return findingMatchesIntent(finding, intent)
	}
	ambiguous := len(defaultInstanceGroup(intent, finding, groups)) > 1
	if finding.Name == "" {
		if ambiguous {
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
	return !ambiguous && len(defaultInstanceGroup(intent, finding, groups)) == 1
}

func defaultInstanceGroup(intent CapabilityIntent, finding Finding, groups map[instanceGroup]map[string]struct{}) map[string]struct{} {
	direction := intent.Direction
	if direction == "" {
		direction = normalizedDirection(finding)
	}
	return groups[instanceGroup{intent.Capability, direction}]
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
