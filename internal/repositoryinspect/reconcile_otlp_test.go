package repositoryinspect

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestReconcileOTLPSignalEvidenceSatisfiesDefaultIntent(t *testing.T) {
	manifest := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        "demo",
		Environment: "dev",
		Telemetry: application.TelemetryRequirements{
			OTLP: &application.OTLPRequirement{Signals: []string{"traces"}},
		},
	}
	findings := []Finding{{
		Capability: "telemetry.otlp",
		Name:       "traces",
		Direction:  DirectionExport,
		Confidence: ConfidenceSuggested,
		Evidence: []Evidence{{
			Kind:   EvidenceImport,
			Path:   "main.go",
			Detail: "source references OTLP trace export",
		}},
	}}
	_, items := Reconcile(findings, &manifest)
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].State != ReconciliationSatisfied {
		t.Fatalf("state = %q, want %q: %#v", items[0].State, ReconciliationSatisfied, items[0])
	}
}
