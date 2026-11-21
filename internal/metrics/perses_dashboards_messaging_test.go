package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestPersesMessagingCatalogIsUnavailableWithoutNativeSignal(t *testing.T) {
	if len(capability.RabbitMQIntegration.Observability.Signals) != 0 {
		t.Fatalf("RabbitMQ integration unexpectedly declares observability signals: %+v", capability.RabbitMQIntegration.Observability.Signals)
	}
	catalog := DefaultPersesMessagingCatalog().DashboardCatalog()
	if len(catalog) != 1 {
		t.Fatalf("messaging catalog must contain one conditional entry, got %d", len(catalog))
	}
	entry, ok := catalog[PersesMessagingDashboardKey]
	if !ok {
		t.Fatalf("messaging catalog is missing %q", PersesMessagingDashboardKey)
	}
	if !entry.Unavailable {
		t.Fatal("RabbitMQ dashboard must remain unavailable without a native signal")
	}
	if entry.Backend != string(capability.ProviderRabbitMQ) {
		t.Fatalf("backend = %q, want %q", entry.Backend, capability.ProviderRabbitMQ)
	}
	if !strings.Contains(entry.Reason, "RabbitMQ") || !strings.Contains(entry.Reason, "no exporter is installed") {
		t.Fatalf("unavailable reason must explain the missing RabbitMQ signal and exporter boundary: %q", entry.Reason)
	}
}

func TestPersesMessagingEligibilityFailsClosedForSelectionAndInventories(t *testing.T) {
	catalog := DefaultPersesMessagingCatalog()
	cases := []struct {
		name           string
		persesSelected bool
		signals        map[string]bool
	}{
		{name: "unselected"},
		{name: "selected without signal", persesSelected: true},
		{name: "fabricated RabbitMQ signal", persesSelected: true, signals: map[string]bool{"rabbitmq": true}},
		{name: "unrelated real signal", persesSelected: true, signals: map[string]bool{"prometheus": true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := catalog.EligibilityFor(tc.persesSelected, tc.signals)
			if got.Available != PersesMessagingDashboardUnavailable {
				t.Fatalf("availability = %q, want unavailable", got.Available)
			}
			if got.Key != PersesMessagingDashboardKey || got.Backend != string(capability.ProviderRabbitMQ) {
				t.Fatalf("unexpected catalog identity: %+v", got)
			}
			if got.Reason == "" {
				t.Fatal("unavailable result must include a reason")
			}
		})
	}
}

func TestPersesMessagingEligibilityFailsClosedWithoutPinnedRelease(t *testing.T) {
	got := (PersesMessagingCatalog{}).EligibilityFor(true, map[string]bool{"rabbitmq": true})
	if got.Available != PersesMessagingDashboardUnavailable {
		t.Fatalf("availability = %q, want unavailable", got.Available)
	}
	if !strings.Contains(got.Reason, "release is not pinned") {
		t.Fatalf("reason = %q, want unpinned-release explanation", got.Reason)
	}
}

func TestPersesMessagingNativeSignalRequiresBackendVerification(t *testing.T) {
	base := capability.ProviderObservabilitySignal{
		Kind:   capability.ObservabilityMetrics,
		Status: capability.ObservabilitySupported,
		Mode:   capability.ObservabilityNative,
	}
	if hasVerifiedNativeMetrics([]capability.ProviderObservabilitySignal{{
		Name:         "rabbitmq",
		Kind:         base.Kind,
		Status:       base.Status,
		Mode:         base.Mode,
		Verification: capability.ObservabilityVerifyNone,
	}}) {
		t.Fatal("unverified native metrics must not unlock a messaging dashboard")
	}
	if !hasVerifiedNativeMetrics([]capability.ProviderObservabilitySignal{{
		Name:         "rabbitmq",
		Kind:         base.Kind,
		Status:       base.Status,
		Mode:         base.Mode,
		Verification: capability.ObservabilityVerifyBackend,
	}}) {
		t.Fatal("backend-verified native metrics should satisfy the signal predicate")
	}
}

func TestPersesMessagingUnavailableJSONMatchesGolden(t *testing.T) {
	first, err := PersesMessagingDashboardCatalogJSON()
	if err != nil {
		t.Fatalf("marshal dashboard availability: %v", err)
	}
	second, err := PersesMessagingDashboardCatalogJSON()
	if err != nil {
		t.Fatalf("marshal dashboard availability again: %v", err)
	}
	if string(first) != string(second) {
		t.Fatal("messaging dashboard availability JSON is not deterministic")
	}
	want, err := os.ReadFile(filepath.Join("testdata", "perses-dashboard-messaging", "rabbitmq-messaging.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if string(first) != string(want) {
		t.Fatalf("messaging dashboard availability differs from golden:\n got: %s\nwant: %s", first, want)
	}
	var doc PersesMessagingDashboardUnavailableDocument
	if err := json.Unmarshal(first, &doc); err != nil {
		t.Fatalf("decode unavailable document: %v", err)
	}
	if doc.Kind != PersesMessagingUnavailableKind || !doc.Unavailable || doc.Key != PersesMessagingDashboardKey {
		t.Fatalf("unexpected unavailable document: %+v", doc)
	}
	if strings.Contains(string(first), "datasources") || strings.Contains(string(first), "panels") ||
		strings.Contains(string(first), "tenant") || strings.Contains(string(first), "http://") || strings.Contains(string(first), "https://") {
		t.Fatalf("unavailable dashboard document must not carry a datasource, panel, tenant identity or endpoint: %s", first)
	}
}

func TestPersesMessagingAvailabilityMapIsExplicit(t *testing.T) {
	got := PersesMessagingDashboardAvailability()
	if len(got) != 1 {
		t.Fatalf("availability map must contain one entry, got %d", len(got))
	}
	entry, ok := got[PersesMessagingDashboardKey]
	if !ok || entry.Available != PersesMessagingDashboardUnavailable || entry.Reason == "" {
		t.Fatalf("unexpected availability entry: %+v", entry)
	}
}
