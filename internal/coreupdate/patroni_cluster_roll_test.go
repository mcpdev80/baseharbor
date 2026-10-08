package coreupdate

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeDCS struct{ valid bool }

func (f fakeDCS) Snapshot(context.Context) (DCSRecoveryEvidence, error) {
	return DCSRecoveryEvidence{}, ErrDCSUnsupported
}
func (f fakeDCS) Validate(context.Context, DCSRecoveryEvidence) error {
	if !f.valid {
		return errors.New("invalid")
	}
	return nil
}
func (f fakeDCS) VerifyRestorable(context.Context, DCSRecoveryEvidence) error {
	if !f.valid {
		return errors.New("unrestorable")
	}
	return nil
}
func (f fakeDCS) Restore(context.Context, DCSRecoveryEvidence) error { return ErrDCSUnsupported }

type fakeClusterRoll struct {
	resumablePatroniFake
}

func (f *fakeClusterRoll) Switchover(_ context.Context, old, next string) error {
	f.calls = append(f.calls, "switch:"+old+":"+next)
	for i := range f.members {
		if f.members[i].Name == old {
			f.members[i].Primary = false
			f.members[i].Replica = true
		}
		if f.members[i].Name == next {
			f.members[i].Primary = true
			f.members[i].Replica = false
		}
	}
	return nil
}
func TestPatroniClusterRollNeedsDCSEvidenceBeforeMutation(t *testing.T) {
	f := &fakeClusterRoll{resumablePatroniFake: resumablePatroniFake{fakePatroniRoll: fakePatroniRoll{members: []PatroniMemberState{{Name: "pg1", Primary: true, Healthy: true}, {Name: "pg2", Replica: true, Healthy: true}, {Name: "pg3", Replica: true, Healthy: true}}}, steps: map[string]string{}}}
	err := RollPatroniCluster(context.Background(), f, nil, DCSRecoveryEvidence{}, "core", "cluster", "0.4.24", 0)
	if !errors.Is(err, ErrDCSUnsupported) || len(f.calls) != 0 {
		t.Fatalf("missing DCS gate allowed progress: %v calls=%v", err, f.calls)
	}
	evidence := DCSRecoveryEvidence{Installation: "core", Cluster: "cluster", Release: "0.4.24", SnapshotID: "backup", SHA256: strings.Repeat("a", 64)}
	if err := RollPatroniCluster(context.Background(), f, fakeDCS{valid: false}, evidence, "core", "cluster", "0.4.24", 0); err == nil || len(f.calls) != 0 {
		t.Fatalf("invalid DCS permitted action: %v", err)
	}
	if err := RollPatroniCluster(context.Background(), f, fakeDCS{valid: true}, evidence, "core", "cluster", "0.4.24", 0); err != nil {
		t.Fatal(err)
	}
	actions := strings.Join(f.calls, ",")
	if !strings.Contains(actions, "recreate:pg2") || !strings.Contains(actions, "recreate:pg3") || !strings.Contains(actions, "switch:pg1:pg2") || !strings.Contains(actions, "recreate:pg1") {
		t.Fatalf("missing staged orchestration: %s", actions)
	}
	if strings.Index(actions, "switch:pg1:pg2") < strings.Index(actions, "recreate:pg3") || strings.Index(actions, "recreate:pg1") < strings.Index(actions, "switch:pg1:pg2") {
		t.Fatalf("unsafe sequence: %s", actions)
	}
}

func TestCaptureAndVerifyDCSFailsClosed(t *testing.T) {
	ctx := context.Background()
	if _, err := CaptureAndVerifyDCS(ctx, nil, "core", "cluster", "0.4.24"); !errors.Is(err, ErrDCSUnsupported) {
		t.Fatalf("missing adapter accepted: %v", err)
	}
	if _, err := CaptureAndVerifyDCS(ctx, fakeDCS{valid: true}, "core", "cluster", "0.4.24"); !errors.Is(err, ErrDCSUnsupported) {
		t.Fatalf("adapter without snapshot rejected incorrectly: %v", err)
	}
}

func TestPatroniClusterRejectsAmbiguousPostSwitchoverResume(t *testing.T) {
	members := []PatroniMemberState{{Name: "pg1", Replica: true, Healthy: true}, {Name: "pg2", Primary: true, Healthy: true}, {Name: "pg3", Replica: true, Healthy: true}}
	f := &fakeClusterRoll{resumablePatroniFake: resumablePatroniFake{fakePatroniRoll: fakePatroniRoll{members: members}, steps: map[string]string{"pg1": "applying", "pg2": "verified", "pg3": "verified"}}}
	evidence := DCSRecoveryEvidence{Installation: "core", Cluster: "cluster", Release: "0.4.24", SnapshotID: "backup", SHA256: strings.Repeat("a", 64)}
	err := RollPatroniCluster(context.Background(), f, fakeDCS{valid: true}, evidence, "core", "cluster", "0.4.24", 0)
	if err == nil || !strings.Contains(err.Error(), "original switchover") {
		t.Fatalf("ambiguous leader resume accepted: %v", err)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "recreate:") || strings.HasPrefix(c, "switch:") {
			t.Fatalf("mutated ambiguous cluster: %v", f.calls)
		}
	}
}
