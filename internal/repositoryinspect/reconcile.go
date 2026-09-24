package repositoryinspect

import (
	"sort"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// Reconcile compares read-only repository evidence with the explicit application
// contract. It never mutates the manifest. In particular, "stale" means only
// that current inspection did not rediscover evidence; it is never permission
// to remove a declared capability.
func Reconcile(findings []Finding, manifest *application.Manifest) ([]CapabilityIntent, []ReconciliationItem) {
	declared := declaredCapabilityIntents(manifest)
	items := make([]ReconciliationItem, 0, len(declared)+len(findings))
	matchedFinding := make([]bool, len(findings))

	for _, intent := range declared {
		item := ReconciliationItem{
			Capability: intent.Capability,
			Name:       intent.Name,
			Direction:  intent.Direction,
			State:      ReconciliationStale,
		}
		best := -1
		for i, finding := range findings {
			if !findingMatchesIntent(finding, intent) {
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

func declaredCapabilityIntents(manifest *application.Manifest) []CapabilityIntent {
	if manifest == nil {
		return nil
	}
	var intents []CapabilityIntent
	for _, name := range application.SQLInstanceNames(*manifest) {
		intents = append(intents, CapabilityIntent{Capability: "database.sql", Name: name, Direction: DirectionConsume})
	}
	for _, name := range application.CacheInstanceNames(*manifest) {
		intents = append(intents, CapabilityIntent{Capability: "cache.key-value", Name: name, Direction: DirectionConsume})
	}
	for _, name := range application.ObjectStorageBucketNames(*manifest) {
		intents = append(intents, CapabilityIntent{Capability: "object-storage.s3", Name: name, Direction: DirectionConsume})
	}
	if manifest.Services.Secrets {
		intents = append(intents, CapabilityIntent{Capability: "secrets", Direction: DirectionConsume})
	}
	for _, exposure := range manifest.Exposures {
		intents = append(intents, CapabilityIntent{Capability: "exposure.http", Name: exposure.Name, Direction: DirectionProvide})
	}
	if application.HasOTLPTelemetry(*manifest) {
		intents = append(intents, CapabilityIntent{Capability: "telemetry.otlp", Name: "default", Direction: DirectionExport})
	}
	for _, source := range manifest.Metrics.Sources {
		intents = append(intents, CapabilityIntent{Capability: "metrics", Name: source.Name, Direction: DirectionProvide})
	}
	return intents
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
