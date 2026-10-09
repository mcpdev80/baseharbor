package coreupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeDCSSwitchover struct {
	calls []string
	fail  string
}

func (o *fakeDCSSwitchover) FenceOldDCS(context.Context) error {
	o.calls = append(o.calls, "fence")
	if o.fail == "fence" {
		return errors.New("fault")
	}
	return nil
}
func (o *fakeDCSSwitchover) VerifyFenced(context.Context) error {
	o.calls = append(o.calls, "verify_fence")
	if o.fail == "verify_fence" {
		return errors.New("fault")
	}
	return nil
}
func (o *fakeDCSSwitchover) ActivateIsolated(context.Context, DCSRecoveryEvidence) error {
	o.calls = append(o.calls, "activate")
	if o.fail == "activate" {
		return errors.New("fault")
	}
	return nil
}
func (o *fakeDCSSwitchover) VerifyNewQuorum(context.Context, DCSRecoveryEvidence) error {
	o.calls = append(o.calls, "quorum")
	if o.fail == "quorum" {
		return errors.New("fault")
	}
	return nil
}
func (o *fakeDCSSwitchover) VerifyPatroniDCS(context.Context) error {
	o.calls = append(o.calls, "patroni")
	if o.fail == "patroni" {
		return errors.New("fault")
	}
	return nil
}
func (o *fakeDCSSwitchover) CommitCutover(context.Context, DCSRecoveryEvidence) error {
	o.calls = append(o.calls, "commit")
	if o.fail == "commit" {
		return errors.New("fault")
	}
	return nil
}

func TestDCSCutoverFencesBeforeActivationAndResumes(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	journal := DCSCutoverJournal{Path: filepath.Join(dir, "cutover")}
	ev := DCSRecoveryEvidence{Installation: "core", Target: "target", Cluster: "cluster", Release: "0.4.24", SnapshotID: "snapshot", SHA256: strings.Repeat("a", 64)}
	ops := &fakeDCSSwitchover{}
	err := RunVerifiedDCSCutover(context.Background(), nil, ev, "core", "target", "cluster", "0.4.24", ops, journal)
	if !errors.Is(err, ErrDCSUnsupported) || len(ops.calls) != 0 {
		t.Fatalf("unverified snapshot triggered mutations: %v %v", err, ops.calls)
	}
	if err := RunVerifiedDCSCutover(context.Background(), fakeDCS{valid: true}, ev, "core", "target", "cluster", "0.4.24", ops, journal); err != nil {
		t.Fatal(err)
	}
	order := strings.Join(ops.calls, ",")
	if !strings.HasPrefix(order, "fence,verify_fence,verify_fence,activate,quorum,patroni,commit") {
		t.Fatalf("unsafe order: %s", order)
	}
	other := ev
	other.SnapshotID = "second-valid-snapshot"
	if err := RunVerifiedDCSCutover(context.Background(), fakeDCS{valid: true}, other, "core", "target", "cluster", "0.4.24", ops, journal); err == nil {
		t.Fatal("different snapshot resumed fenced cutover")
	}
	if phase, err := journal.load(); err != nil || phase != "committed" {
		t.Fatalf("cutover not durable: %s %v", phase, err)
	}
	before := len(ops.calls)
	if err := RunVerifiedDCSCutover(context.Background(), fakeDCS{valid: true}, ev, "core", "target", "cluster", "0.4.24", ops, journal); err != nil {
		t.Fatal(err)
	}
	for _, call := range ops.calls[before:] {
		if call == "fence" || call == "activate" || call == "verify_fence" || call == "commit" {
			t.Fatalf("resumed committed cutover replayed obsolete operation: %v", ops.calls[before:])
		}
	}
	if got := strings.Join(ops.calls[before:], ","); got != "quorum,patroni" {
		t.Fatalf("committed recovery must validate active DCS and Patroni only, got %s", got)
	}
}
func TestDCSCutoverRefusesAmbiguousFence(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	journal := DCSCutoverJournal{Path: filepath.Join(dir, "cutover")}
	ev := DCSRecoveryEvidence{Installation: "core", Target: "target", Cluster: "cluster", Release: "0.4.24", SnapshotID: "snapshot", SHA256: strings.Repeat("a", 64)}
	ops := &fakeDCSSwitchover{fail: "fence"}
	if err := RunVerifiedDCSCutover(context.Background(), fakeDCS{valid: true}, ev, "core", "target", "cluster", "0.4.24", ops, journal); err == nil {
		t.Fatal("failed fence allowed progress")
	}
	ops.fail = ""
	if err := RunVerifiedDCSCutover(context.Background(), fakeDCS{valid: true}, ev, "core", "target", "cluster", "0.4.24", ops, journal); err == nil {
		t.Fatal("ambiguous previous fence retried")
	}
	if strings.Contains(strings.Join(ops.calls, ","), "activate") {
		t.Fatal("new DCS activated without fence proof")
	}
}
