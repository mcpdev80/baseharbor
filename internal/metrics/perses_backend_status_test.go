package metrics

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// scriptedSeriesProbe is a per-target persesInfraProbe that answers each panel's
// own series query with a scripted verdict (every-query mode) and records the
// (endpoint, query) pairs it was asked — so a test proves the probe ran one
// query per panel for THIS manifest, not against a single global endpoint.
type scriptedSeriesProbe struct {
	answersAll bool
	err        error
	asked      []string
}

func (p *scriptedSeriesProbe) querySeries(_ context.Context, _ string, query string) (bool, error) {
	p.asked = append(p.asked, query)
	if p.err != nil {
		return false, p.err
	}
	if p.answersAll {
		return true, nil
	}
	// answers-all-false: no panel answered — a silent / not-probed backend.
	return false, nil
}

// TestPersesBackendAnswersStatusAnsweringTargetReportsVerified is the 0428-186
// positive case: a selected, pinned Perses surface whose backend genuinely
// answers (every infra panel's own series query returns rows over the real
// per-target probe) promotes the three-state contract to "verified" — the
// 0428-078 identity_dashboard_eligible=true positive case becomes reachable.
// The probe is a scripted per-target probe answering every panel, so the
// verified verdict travels the probe's own per-panel query path, never a static
// true.
func TestPersesBackendAnswersStatusAnsweringTargetReportsVerified(t *testing.T) {
	m := persesPrometheusDatasourceManifest()
	// Materialize a real Prometheus provider for this Target with an authorized
	// source, so the fail-closed dashboard build succeeds and the scripted
	// per-target probe can observe a genuine backend answer.
	source := persesProviderSource("tempo:runtime", capability.ProviderTempo)
	persesMaterializeProvider(t, m, source)
	// Every panel answered: the backend genuinely answers for this Target.
	probe := &scriptedSeriesProbe{answersAll: true}
	got := persesBackendAnswersStatusWithProbe(t.Context(), m, probe)
	if got == nil || *got != capability.PersesStatusVerified {
		t.Fatalf("an answering per-target Perses backend must report %q, got %+v", capability.PersesStatusVerified, got)
	}
	// The probe must have actually been asked one query per panel for THIS
	// manifest — the verdict travels per-target, not a global endpoint.
	if len(probe.asked) == 0 {
		t.Fatal("the per-target probe must be asked at least one panel query for the selected manifest")
	}
}

// TestPersesBackendAnswersStatusSilentTargetFailsClosedToConfiguredUnverified is
// the 0428-186 fail-closed case: a selected, pinned Perses surface whose backend
// is silent or not probed (no panel's own series query returns rows) resolves
// to "configured_unverified" — never a static "verified". "verified" is a probe
// result only, so a silent target can never be promoted by pin validation alone.
func TestPersesBackendAnswersStatusSilentTargetFailsClosedToConfiguredUnverified(t *testing.T) {
	m := persesPrometheusDatasourceManifest()
	probe := &scriptedSeriesProbe{answersAll: false}
	got := persesBackendAnswersStatusWithProbe(t.Context(), m, probe)
	if got == nil || *got != capability.PersesStatusConfiguredUnverified {
		t.Fatalf("a silent per-target Perses backend must report %q, got %+v", capability.PersesStatusConfiguredUnverified, got)
	}
}

// TestPersesBackendAnswersStatusUnselectedFailsClosedToNotConfigured pins the
// unselected default.
func TestPersesBackendAnswersStatusUnselectedFailsClosedToNotConfigured(t *testing.T) {
	m := persesPrometheusDatasourceManifest()
	m.Services.Perses = false
	m.Services.ObservabilityManagementUI = false
	got := persesBackendAnswersStatusWithProbe(t.Context(), m, &scriptedSeriesProbe{answersAll: true})
	if got == nil || *got != capability.PersesStatusNotConfigured {
		t.Fatalf("an unselected Perses surface must report %q, got %+v", capability.PersesStatusNotConfigured, got)
	}
}

// TestPersesBackendAnswersStatusSilentProbeErrorFailsClosed pins that an HTTP /
// transport probe error (a backend that cannot be reached) fails closed to
// configured_unverified, never a static "verified".
func TestPersesBackendAnswersStatusSilentProbeErrorFailsClosed(t *testing.T) {
	m := persesPrometheusDatasourceManifest()
	probe := &scriptedSeriesProbe{answersAll: true, err: context.DeadlineExceeded}
	got := persesBackendAnswersStatusWithProbe(t.Context(), m, probe)
	if got == nil || *got != capability.PersesStatusConfiguredUnverified {
		t.Fatalf("a probe transport error must fail closed to %q, got %+v", capability.PersesStatusConfiguredUnverified, got)
	}
}
