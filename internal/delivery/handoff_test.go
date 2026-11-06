package delivery

import (
	"encoding/json"
	"strings"
	"testing"
)

// delegatedSelection is the fixture selection for the delegated direction:
// an external reconciler (Argo CD style provider id) owns reconciliation.
func delegatedSelection() Selection {
	return Selection{
		Mode:       ModeDelegated,
		Provider:   "gitops/argocd",
		Owner:      OwnerExternal,
		Placement:  PlacementExternal,
		Revision:   "gitops-rev-42",
		Provenance: "git@example:team/app-desired.git@42",
	}
}

// foreignTracking is the external reconciler's own tracking metadata that a
// handoff must carry through untouched.
func foreignTracking() TrackingMetadata {
	return TrackingMetadata{
		"argocd.argoproj.io/tracking-id":    "web:apps/Deployment/prod/web",
		"helm.sh/release":                   "web-prod",
		"kustomize.config.k8s.io/commonlab": "team-web",
	}
}

func directRequest(op string) HandoffRequest {
	return HandoffRequest{
		Mode:      ModeDirect,
		Operation: op,
		Selection: Direct(),
	}
}

func delegatedRequest(op string, tracking TrackingMetadata, drift DriftObservation) HandoffRequest {
	return HandoffRequest{
		Mode:      ModeDelegated,
		Operation: op,
		Selection: delegatedSelection(),
		Tracking:  tracking,
		Drift:     drift,
	}
}

func inSyncDrift() DriftObservation {
	return DriftObservation{
		DesiredDigest:  "sha256:aaa0",
		ObservedDigest: "sha256:aaa0",
		Evidence:       "argocd:sync-revision=42",
		Synced:         true,
		Healthy:        true,
	}
}

func driftedObservation() DriftObservation {
	return DriftObservation{
		DesiredDigest:  "sha256:bbb1",
		ObservedDigest: "sha256:aaa0",
		Evidence:       "argocd:sync-revision=42",
	}
}

// TestHandoffDirectionActionMatrix is the owner/render fixture table: BOTH
// directions (direct AND delegated) x all four actions
// (promote/apply/repair/destroy). It pins who acts and who only observes.
func TestHandoffDirectionActionMatrix(t *testing.T) {
	cases := []struct {
		direction  string
		op         string
		req        HandoffRequest
		wantAction string
		wantOwner  string
	}{
		{"direct", HandoffPromote, directRequest(HandoffPromote), "observe", "baseharbor"},
		{"direct", HandoffApply, directRequest(HandoffApply), "observe", "baseharbor"},
		{"direct", HandoffRepair, directRequest(HandoffRepair), "observe", "baseharbor"},
		{"direct", HandoffDestroy, directRequest(HandoffDestroy), "observe", "baseharbor"},
		{"delegated", HandoffPromote, delegatedRequest(HandoffPromote, foreignTracking(), inSyncDrift()), "observe", "external"},
		{"delegated", HandoffApply, delegatedRequest(HandoffApply, foreignTracking(), inSyncDrift()), "observe", "external"},
		{"delegated", HandoffRepair, delegatedRequest(HandoffRepair, foreignTracking(), inSyncDrift()), "observe", "external"},
		{"delegated", HandoffDestroy, delegatedRequest(HandoffDestroy, foreignTracking(), inSyncDrift()), "observe", "external"},
	}
	for _, tc := range cases {
		t.Run(tc.direction+"/"+tc.op, func(t *testing.T) {
			got, err := HandOff(tc.req)
			if err != nil {
				t.Fatalf("%s/%s handoff: %v", tc.direction, tc.op, err)
			}
			if got.Action != tc.wantAction {
				t.Fatalf("%s/%s action = %q, want %q", tc.direction, tc.op, got.Action, tc.wantAction)
			}
			if got.Owner != tc.wantOwner {
				t.Fatalf("%s/%s owner = %q, want %q", tc.direction, tc.op, got.Owner, tc.wantOwner)
			}
			if got.Drifted {
				t.Fatalf("%s/%s must not report drift in the happy path", tc.direction, tc.op)
			}
		})
	}
}

// TestDelegatedHandoffPreservesForeignTrackingMetadata is the ERHALT-Zelle:
// the foreign tracking metadata survives the handoff byte-equal, and the
// preservation check below is the line a mutation must turn red.
func TestDelegatedHandoffPreservesForeignTrackingMetadata(t *testing.T) {
	before := foreignTracking()
	got, err := HandOff(delegatedRequest(HandoffApply, before, inSyncDrift()))
	if err != nil {
		t.Fatal(err)
	}
	if diff := DiffTracking(before, got.Tracking); !diff.Empty() {
		t.Fatalf("delegated handoff lost foreign tracking metadata: lost=%v changed=%v added=%v", diff.Lost, diff.Changed, diff.Added)
	}
	if len(got.Tracking) != len(before) {
		t.Fatalf("tracking metadata size = %d, want %d", len(got.Tracking), len(before))
	}
	// The rendered read-model round-trips the metadata, too: nothing in the
	// JSON display path may drop it. This is the ERHALT-ZELLE: deleting the
	// preservation assertion below must turn the mutant RED.
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for key := range before {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("rendered handoff dropped tracking key %q: %s", key, raw)
		}
	}
}

// TestDirectHandoffReportsForeignTrackingMetadata: direct mode keeps its own
// semantics but a handoff never silently drops metadata it was handed.
func TestDirectHandoffReportsForeignTrackingMetadata(t *testing.T) {
	before := foreignTracking()
	req := directRequest(HandoffRepair)
	req.Tracking = before
	got, err := HandOff(req)
	if err != nil {
		t.Fatal(err)
	}
	if diff := DiffTracking(before, got.Tracking); !diff.Empty() {
		t.Fatalf("direct handoff altered tracking metadata: %+v", diff)
	}
}

// TestDelegatedDriftRefusesWithoutSilentFallback is the drift cell: an
// external drift leads to an explicit refusal, never a quiet direct apply.
func TestDelegatedDriftRefusesWithoutSilentFallback(t *testing.T) {
	got, err := HandOff(delegatedRequest(HandoffApply, foreignTracking(), driftedObservation()))
	if err != nil {
		t.Fatal(err)
	}
	if got.Action != "refused" {
		t.Fatalf("drifted delegated handoff action = %q, want refused (no silent fallback)", got.Action)
	}
	if !got.Drifted {
		t.Fatal("drifted handoff must report drift")
	}
	if got.Synced || got.Healthy {
		t.Fatal("a drifted resource never renders synced/healthy")
	}
	if got.Mode != ModeDelegated || got.Owner != string(OwnerExternal) {
		t.Fatalf("refusal must stay delegated: mode=%s owner=%s", got.Mode, got.Owner)
	}
	if !strings.Contains(got.Reason, "no silent fallback") {
		t.Fatalf("reason must name the boundary, got %q", got.Reason)
	}
	// The refusal still preserves the external metadata for the operator.
	if diff := DiffTracking(foreignTracking(), got.Tracking); !diff.Empty() {
		t.Fatalf("refused handoff lost tracking metadata: %+v", diff)
	}
}

// TestDriftClaimWithoutExternalEvidenceFailsClosed: digests alone are not
// external evidence; the handoff refuses to decide on them.
func TestDriftClaimWithoutExternalEvidenceFailsClosed(t *testing.T) {
	drift := driftedObservation()
	drift.Evidence = ""
	if _, err := HandOff(delegatedRequest(HandoffRepair, nil, drift)); err == nil {
		t.Fatal("drift asserted without external evidence must fail closed")
	}
}

// TestMalformedTrackingMetadataFailsClosed pins that empty keys/values are
// integrity errors, not silently dropped entries.
func TestMalformedTrackingMetadataFailsClosed(t *testing.T) {
	cases := []TrackingMetadata{
		{"": "value"},
		{"key": ""},
		{"  ": "value"},
	}
	for i, tracking := range cases {
		req := delegatedRequest(HandoffApply, tracking, inSyncDrift())
		if _, err := HandOff(req); err == nil {
			t.Fatalf("case %d: malformed tracking metadata %v accepted", i, tracking)
		}
	}
}

// TestHandoffRejectsUnsupportedShapes: mode/selection/op mismatches fail
// closed exactly like the selection contract they re-validate.
func TestHandoffRejectsUnsupportedShapes(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*HandoffRequest)
	}{
		{"unknown operation", func(r *HandoffRequest) { r.Operation = "rollout" }},
		{"unknown mode", func(r *HandoffRequest) { r.Mode = Mode("sidecar") }},
		{"mode contradicts selection", func(r *HandoffRequest) { r.Mode = ModeDirect }},
		{"selection contradicts mode", func(r *HandoffRequest) { r.Selection = Direct() }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := delegatedRequest(HandoffApply, nil, DriftObservation{})
			tc.mutate(&req)
			if _, err := HandOff(req); err == nil {
				t.Fatal("expected fail-closed validation error")
			}
		})
	}
}

// TestHandoffRenderIsBounded proves the read-model renders as bounded JSON
// with the pinned fields and nothing secret-like.
func TestHandoffRenderIsBounded(t *testing.T) {
	got, err := HandOff(delegatedRequest(HandoffDestroy, foreignTracking(), driftedObservation()))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var view map[string]any
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"mode", "operation", "owner", "action", "drifted", "reason", "tracking_metadata"} {
		if _, ok := view[key]; !ok {
			t.Fatalf("render is missing bounded field %q: %s", key, raw)
		}
	}
}
