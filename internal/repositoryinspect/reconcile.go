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

	for _, intent := range declared {
		aliasName, aliasAmbiguous := defaultInstanceAlias(findings, intent)
		item := ReconciliationItem{
			Capability: intent.Capability,
			Name:       intent.Name,
			Direction:  intent.Direction,
			State:      ReconciliationStale,
		}
		best := -1
		for i, finding := range findings {
			matches := findingMatchesIntent(finding, intent)
			if aliasAmbiguous && finding.Name == "" && defaultInstanceIntent(intent) {
				matches = false
			}
			if aliasName != "" && finding.Name == aliasName && findingIdentityCompatible(finding, intent) {
				matches = true
			}
			if !matches {
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

func defaultInstanceAlias(findings []Finding, intent CapabilityIntent) (string, bool) {
	if !defaultInstanceIntent(intent) {
		return "", false
	}

	names := map[string]struct{}{}
	for _, finding := range findings {
		if !findingIdentityCompatible(finding, intent) {
			continue
		}
		if len(nonManifestEvidence(finding.Evidence)) == 0 {
			continue
		}
		if finding.Name == "default" {
			// Exact identity is authoritative; any additional named findings remain
			// separate reconciliation evidence rather than changing its meaning.
			return "", false
		}
		if finding.Name == "" {
			continue
		}
		names[finding.Name] = struct{}{}
	}
	if len(names) == 1 {
		for name := range names {
			return name, false
		}
	}
	return "", len(names) > 1
}

func defaultInstanceIntent(intent CapabilityIntent) bool {
	if intent.Name != "default" {
		return false
	}
	switch intent.Capability {
	case "database.sql", "cache.key-value", "database.document":
		return true
	default:
		return false
	}
}

func findingIdentityCompatible(finding Finding, intent CapabilityIntent) bool {
	if finding.Capability != intent.Capability {
		return false
	}
	direction := normalizedDirection(finding)
	return intent.Direction == "" || direction == "" || direction == intent.Direction
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
