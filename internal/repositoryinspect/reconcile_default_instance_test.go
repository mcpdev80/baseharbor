package repositoryinspect

import (
	"context"
	"testing"
)

// Regression coverage for issue #752. Cases marked (752-historical) are
// reused from commit 70292f0cd436fdeb170652e26e6c5e228da23c1d on the
// diverged fix/752-default-instance-reconcile-v0421 branch; only the test
// intent was carried over, not the branch itself.

func TestReconcileDefaultInstanceAliasesSingleDetectedProvider(t *testing.T) { // (752-historical)
	tests := []struct {
		capability string
		detected   string
	}{
		{capability: "database.sql", detected: "postgres"},
		{capability: "cache.key-value", detected: "valkey"},
		{capability: "database.document", detected: "mongodb"},
	}

	for _, tc := range tests {
		t.Run(tc.capability, func(t *testing.T) {
			_, items := Reconcile(
				[]Finding{{
					Capability: tc.capability,
					Name:       tc.detected,
					Confidence: ConfidenceDetected,
					Evidence:   []Evidence{{Kind: EvidenceCompose, Path: "compose.yaml", Detail: "provider detected"}},
				}},
				[]CapabilityIntent{{
					Capability: tc.capability,
					Name:       "default",
					Direction:  DirectionConsume,
				}},
			)

			if len(items) != 1 {
				t.Fatalf("items = %#v", items)
			}
			if items[0].Name != "default" || items[0].State != ReconciliationSatisfied {
				t.Fatalf("default instance did not reconcile: %#v", items[0])
			}
		})
	}
}

func TestReconcileDefaultInstanceConsumesNamedAndNamelessEvidence(t *testing.T) { // (752-historical)
	_, items := Reconcile(
		[]Finding{
			{
				Capability: "database.sql",
				Name:       "postgres",
				Confidence: ConfidenceDetected,
				Evidence:   []Evidence{{Kind: EvidenceCompose, Path: "compose.yaml", Detail: "postgres service"}},
			},
			{
				Capability: "database.sql",
				Confidence: ConfidenceDetected,
				Evidence:   []Evidence{{Kind: EvidenceEnv, Path: ".env.example", Detail: "variable DATABASE_URL"}},
			},
		},
		[]CapabilityIntent{{
			Capability: "database.sql",
			Name:       "default",
			Direction:  DirectionConsume,
		}},
	)

	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].State != ReconciliationSatisfied || len(items[0].Evidence) != 2 {
		t.Fatalf("unexpected reconciliation: %#v", items[0])
	}
}

func TestReconcileDefaultInstanceDoesNotGuessAcrossMultipleNamedProviders(t *testing.T) { // (752-historical)
	_, items := Reconcile(
		[]Finding{
			{
				Capability: "database.sql",
				Name:       "primary",
				Confidence: ConfidenceDetected,
				Evidence:   []Evidence{{Kind: EvidenceCompose, Path: "compose.yaml", Detail: "primary"}},
			},
			{
				Capability: "database.sql",
				Name:       "analytics",
				Confidence: ConfidenceDetected,
				Evidence:   []Evidence{{Kind: EvidenceCompose, Path: "compose.yaml", Detail: "analytics"}},
			},
			{
				Capability: "database.sql",
				Confidence: ConfidenceDetected,
				Evidence:   []Evidence{{Kind: EvidenceEnv, Path: ".env.example", Detail: "DATABASE_URL"}},
			},
		},
		[]CapabilityIntent{{
			Capability: "database.sql",
			Name:       "default",
			Direction:  DirectionConsume,
		}},
	)

	var stale, namedNew int
	for _, item := range items {
		if item.Capability != "database.sql" {
			continue
		}
		if item.Name == "default" && item.State == ReconciliationStale {
			stale++
		}
		if item.Name != "" && item.Name != "default" && item.State == ReconciliationNew {
			namedNew++
		}
	}
	if stale != 1 || namedNew != 2 {
		t.Fatalf("ambiguous default was guessed: %#v", items)
	}
	for _, item := range items {
		if item.Name == "" && item.Capability == "database.sql" && len(item.Evidence) == 0 {
			t.Fatalf("nameless evidence was concealed: %#v", items)
		}
	}
}

func TestReconcileDefaultInstanceCountsIdentitiesNotEvidenceRows(t *testing.T) {
	_, items := Reconcile(
		[]Finding{
			{
				Capability: "database.sql",
				Name:       "postgres",
				Confidence: ConfidenceDetected,
				Evidence:   []Evidence{{Kind: EvidenceCompose, Path: "compose.yaml", Detail: "service postgres"}},
			},
			{
				Capability: "database.sql",
				Name:       "postgres",
				Confidence: ConfidenceDetected,
				Evidence:   []Evidence{{Kind: EvidenceDependency, Path: "go.mod", Detail: "pq driver"}},
			},
		},
		[]CapabilityIntent{{
			Capability: "database.sql",
			Name:       "default",
			Direction:  DirectionConsume,
		}},
	)

	if len(items) != 1 {
		t.Fatalf("duplicate evidence rows produced ambiguity: %#v", items)
	}
	if items[0].Name != "default" || items[0].State != ReconciliationSatisfied || len(items[0].Evidence) != 2 {
		t.Fatalf("unexpected reconciliation: %#v", items[0])
	}
}

func TestReconcileDefaultInstanceRespectsDirectionBoundary(t *testing.T) {
	_, items := Reconcile(
		[]Finding{{
			Capability: "database.sql",
			Name:       "postgres",
			Direction:  DirectionProvide,
			Confidence: ConfidenceDetected,
			Evidence:   []Evidence{{Kind: EvidenceCompose, Path: "compose.yaml", Detail: "exported provider"}},
		}},
		[]CapabilityIntent{{
			Capability: "database.sql",
			Name:       "default",
			Direction:  DirectionConsume,
		}},
	)

	if len(items) != 2 {
		t.Fatalf("direction mismatch was aliased: %#v", items)
	}
	for _, item := range items {
		switch item.Name {
		case "default":
			if item.State != ReconciliationStale {
				t.Fatalf("declared default satisfied across directions: %#v", item)
			}
		case "postgres":
			if item.Direction != DirectionProvide || item.State == ReconciliationSatisfied {
				t.Fatalf("provide-direction instance changed: %#v", item)
			}
		default:
			t.Fatalf("unexpected item: %#v", item)
		}
	}
}

func TestReconcileExplicitInstanceStillRequiresExactName(t *testing.T) { // (752-historical)
	_, items := Reconcile(
		[]Finding{{
			Capability: "database.sql",
			Name:       "postgres",
			Confidence: ConfidenceDetected,
			Evidence:   []Evidence{{Kind: EvidenceCompose, Path: "compose.yaml", Detail: "postgres"}},
		}},
		[]CapabilityIntent{{
			Capability: "database.sql",
			Name:       "primary",
			Direction:  DirectionConsume,
		}},
	)

	var stalePrimary, newPostgres bool
	for _, item := range items {
		if item.Name == "primary" && item.State == ReconciliationStale {
			stalePrimary = true
		}
		if item.Name == "postgres" && item.State == ReconciliationNew {
			newPostgres = true
		}
	}
	if !stalePrimary || !newPostgres {
		t.Fatalf("explicit instance identity was weakened: %#v", items)
	}
}

func TestReconcileExplicitDefaultNameStillSatisfiesExactIdentity(t *testing.T) {
	_, items := Reconcile(
		[]Finding{{
			Capability: "database.sql",
			Name:       "default",
			Confidence: ConfidenceDetected,
			Evidence:   []Evidence{{Kind: EvidenceCompose, Path: "compose.yaml", Detail: "default instance"}},
		}},
		[]CapabilityIntent{{
			Capability: "database.sql",
			Name:       "default",
			Direction:  DirectionConsume,
		}},
	)

	if len(items) != 1 || items[0].Name != "default" || items[0].State != ReconciliationSatisfied {
		t.Fatalf("explicit default identity regressed: %#v", items)
	}
}

func TestReconcileOTLPDefaultSignalBehaviorUnchanged(t *testing.T) {
	_, items := Reconcile(
		[]Finding{
			{
				Capability: "telemetry.otlp",
				Name:       "traces",
				Direction:  DirectionExport,
				Confidence: ConfidenceDetected,
				Evidence:   []Evidence{{Kind: EvidenceImport, Path: "app.go", Detail: "trace exporter"}},
			},
			{
				Capability: "telemetry.otlp",
				Name:       "metrics",
				Direction:  DirectionExport,
				Confidence: ConfidenceDetected,
				Evidence:   []Evidence{{Kind: EvidenceImport, Path: "app.go", Detail: "metric exporter"}},
			},
		},
		[]CapabilityIntent{{
			Capability: "telemetry.otlp",
			Name:       "default",
			Direction:  DirectionExport,
		}},
	)

	if len(items) != 1 || items[0].Name != "default" || items[0].State != ReconciliationSatisfied {
		t.Fatalf("OTLP multi-signal default behavior changed: %#v", items)
	}
}

func TestInspectReconcilesQuickInitDefaultSQLAndCacheWithComposeProviders(t *testing.T) { // (752-historical)
	root := t.TempDir()
	writeTestFile(t, root, "baseharbor.yaml", `version: 1
app:
  name: default-instance-reconcile
  environment: dev
services:
  sql:
    enabled: true
  cache:
    enabled: true
`)
	writeTestFile(t, root, "compose.yaml", `services:
  app:
    image: example/app:1
  postgres:
    image: postgres:17
  valkey:
    image: valkey/valkey:8
`)

	result, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}

	for _, capability := range []string{"database.sql", "cache.key-value"} {
		var matching []ReconciliationItem
		for _, item := range result.Reconciliation {
			if item.Capability == capability {
				matching = append(matching, item)
			}
		}
		if len(matching) != 1 {
			t.Fatalf("%s reconciliation = %#v", capability, matching)
		}
		if matching[0].Name != "default" || matching[0].State != ReconciliationSatisfied {
			t.Fatalf("%s reconciliation = %#v", capability, matching[0])
		}
	}
}
