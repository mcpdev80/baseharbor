package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderPersesMessagingDeterministic(t *testing.T) {
	a, err := RenderPersesMessagingDashboard()
	if err != nil {
		t.Fatalf("RenderPersesMessagingDashboard: %v", err)
	}
	b, err := RenderPersesMessagingDashboard()
	if err != nil {
		t.Fatalf("RenderPersesMessagingDashboard: %v", err)
	}
	if string(a) != string(b) {
		t.Fatalf("render is not deterministic")
	}

	wantPath := filepath.Join("testdata", "perses", "rabbitmq-messaging.json")
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", wantPath, err)
	}
	if string(a) != string(want) {
		t.Fatalf("golden mismatch:\n got: %s\nwant: %s", a, want)
	}
}

// TestVerifyMessagingDashboardCatalogBuilds pins the shipped catalog: it builds
// and reports exactly one UNAVAILABLE messaging dashboard, never an eligible one.
func TestVerifyMessagingDashboardCatalogBuilds(t *testing.T) {
	if err := VerifyMessagingDashboardCatalogBuilds(); err != nil {
		t.Fatalf("shipped messaging catalog must build: %v", err)
	}
}

// TestMessagingDashboardCatalogAlwaysUnavailable is the core claim: messaging
// ships no native Perses signal, so the dashboard is UNAVAILABLE for every
// selection, release, and signal inventory.
func TestMessagingDashboardCatalogAlwaysUnavailable(t *testing.T) {
	catalog := DefaultMessagingDashboardCatalog()
	if catalog.Release() != "v0.55.1" {
		t.Fatalf("catalog release = %q, want v0.55.1", catalog.Release())
	}

	cases := []struct {
		name           string
		persesSelected bool
		signals        map[string]bool
	}{
		{"unselected", false, map[string]bool{"rabbitmq": true}},
		{"selected but no signal", true, map[string]bool{}},
		{"selected with fabricated signal", true, map[string]bool{"rabbitmq": true}},
		{"selected with unrelated signals", true, map[string]bool{"prometheus": true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			elig, err := catalog.EligibleFor(tc.persesSelected, tc.signals)
			if err != nil {
				t.Fatalf("EligibleFor: %v", err)
			}
			if elig.Dashboard != "rabbitmq-messaging" {
				t.Fatalf("dashboard key = %q, want rabbitmq-messaging", elig.Dashboard)
			}
			if elig.VersionPinned != "v0.55.1" {
				t.Fatalf("version = %q, want v0.55.1", elig.VersionPinned)
			}
			if elig.Eligible {
				t.Fatalf("messaging dashboard must never be eligible: %+v", elig)
			}
			if elig.Status != "unavailable" {
				t.Fatalf("status = %q, want unavailable", elig.Status)
			}
			if elig.Reason == "" {
				t.Fatalf("ineligible verdict must carry a concrete reason")
			}
			if elig.Backend != "rabbitmq" {
				t.Fatalf("backend = %q, want rabbitmq", elig.Backend)
			}
		})
	}
}

// TestMessagingDashboardFailClosedOnUnpinnedRelease proves the fail-closed path:
// an empty release contract yields an UNAVAILABLE verdict with a fail-closed
// reason, never an eligible one.
func TestMessagingDashboardFailClosedOnUnpinnedRelease(t *testing.T) {
	catalog := PersesMessagingCatalog{release: "v0.55.1"}
	saved := persesDefaultRelease
	defer func() { persesDefaultRelease = saved }()
	persesDefaultRelease = struct {
		Version                 string
		Image                   string
		PrometheusPluginVersion string
		AuthenticationClass     string
	}{} // empty: not pinned

	elig, err := catalog.EligibleFor(true, map[string]bool{"rabbitmq": true})
	if err != nil {
		t.Fatalf("EligibleFor: %v", err)
	}
	if elig.Eligible {
		t.Fatalf("must fail closed on an unpinned release")
	}
	if elig.Status != "unavailable" {
		t.Fatalf("status = %q, want unavailable", elig.Status)
	}
}

// TestMessagingDashboardRenderIsEmptyPanels pins the contract: the rendered
// dashboard carries no live panels and no datasources, because messaging has no
// native Perses signal to back them.
func TestMessagingDashboardRenderIsEmptyPanels(t *testing.T) {
	data, err := RenderPersesMessagingDashboard()
	if err != nil {
		t.Fatalf("RenderPersesMessagingDashboard: %v", err)
	}
	var dashboard map[string]any
	if err := json.Unmarshal(data, &dashboard); err != nil {
		t.Fatalf("rendered JSON must be valid: %v", err)
	}
	spec, ok := dashboard["spec"].(map[string]any)
	if !ok {
		t.Fatalf("spec missing from rendered dashboard")
	}
	panels, hasPanels := spec["panels"]
	if hasPanels {
		if p, ok := panels.(map[string]any); !ok || len(p) != 0 {
			t.Fatalf("messaging dashboard must render no panels: %v", panels)
		}
	}
	datasources, hasDatasources := spec["datasources"]
	if hasDatasources {
		if d, ok := datasources.(map[string]any); !ok || len(d) != 0 {
			t.Fatalf("messaging dashboard must render no datasources: %v", datasources)
		}
	}
	if dashboard["apiVersion"] != "perses.dev/v1alpha1" {
		t.Fatalf("apiVersion = %v, want perses.dev/v1alpha1", dashboard["apiVersion"])
	}
	if dashboard["kind"] != "PersesDashboard" {
		t.Fatalf("kind = %v, want PersesDashboard", dashboard["kind"])
	}
}
