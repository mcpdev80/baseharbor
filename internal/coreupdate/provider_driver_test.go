package coreupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type mockNativeOps struct {
	quiesce, pinned, original, verified int
	failPinned                          bool
}

func (o *mockNativeOps) Preflight(context.Context, Plan) error { return nil }
func (o *mockNativeOps) Quiesce(context.Context, Delta) error  { o.quiesce++; return nil }
func (o *mockNativeOps) ReconcilePinned(context.Context, Delta) error {
	o.pinned++
	if o.failPinned {
		return errors.New("provider restart failed")
	}
	return nil
}
func (o *mockNativeOps) ReconcileOriginal(context.Context, Delta) error { o.original++; return nil }
func (o *mockNativeOps) VerifySemantics(context.Context, Delta) error   { o.verified++; return nil }
func (o *mockNativeOps) Record(context.Context, Delta, string) error    { return nil }

func TestNativeProviderUpdateStagesBacksUpAndVerifies(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	compose := filepath.Join(dir, "provider-compose.yaml")
	if err := os.WriteFile(compose, []byte("services:\n  postgres-member-1:\n    image: postgres:18.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	installed := Realization{Kind: SQL, Installation: "c", Scope: "shared", Instance: "postgres-member-1", Owner: "baseharbor", Image: "postgres:18.0", Version: "18.0", Digest: digestA}
	target := Desired{Kind: SQL, Image: "postgres:18.2", Version: "18.2", Digest: digestB}
	delta := Delta{Installed: installed, Desired: target, Classification: BackupRequired}
	rt := &snapshotRuntime{data: []byte("postgres-before-upgrade")}
	ops := &mockNativeOps{}
	assets := map[string]NativeProviderAssets{JournalKey(delta): {
		Recovery: VolumeRecovery{Runtime: rt, Directory: filepath.Join(dir, "recovery"), Project: "owned", Volume: "pg-volume", VerifyQuiesced: func(context.Context, string, string) error { return nil }},
		Compose:  ComposeCheckpoint{Path: compose, Directory: filepath.Join(dir, "checkpoints")},
	}}
	journal := filepath.Join(dir, "journal.json")
	if err := RunNativeProviderUpdates(context.Background(), Plan{Release: "0.4.24", Deltas: []Delta{delta}}, journal, ops, assets); err != nil {
		t.Fatal(err)
	}
	if ops.quiesce != 1 || ops.pinned != 1 || ops.verified != 1 || rt.exports != 1 {
		t.Fatalf("native lifecycle incomplete: ops=%+v exports=%d", ops, rt.exports)
	}
	pinned, err := os.ReadFile(compose)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pinned), "postgres:18.2@"+digestB) {
		t.Fatalf("image not pinned: %s", pinned)
	}
	if err := RunNativeProviderUpdates(context.Background(), Plan{Release: "0.4.24", Deltas: []Delta{delta}}, journal, ops, assets); err != nil {
		t.Fatal(err)
	}
	if ops.pinned != 1 || ops.verified != 2 || rt.exports != 1 {
		t.Fatalf("verified step replayed mutation: %+v", ops)
	}
}
func TestNativeProviderUpdateRejectsMissingRecoveryBeforeMutation(t *testing.T) {
	delta := Delta{Installed: Realization{Kind: Secrets, Installation: "c", Scope: "shared", Instance: "openbao-member-1", Owner: "baseharbor", Image: "bao:2.6.0", Version: "2.6.0", Digest: digestA}, Desired: Desired{Kind: Secrets, Image: "bao:2.7.0", Version: "2.7.0", Digest: digestB}, Classification: BackupRequired}
	ops := &mockNativeOps{}
	if err := RunNativeProviderUpdates(context.Background(), Plan{Release: "0.4.24", Deltas: []Delta{delta}}, "journal", ops, nil); err == nil {
		t.Fatal("unbacked OpenBao mutation allowed")
	}
	if ops.quiesce != 0 || ops.pinned != 0 {
		t.Fatal("unverified mutation touched native provider")
	}
}

func TestNativeProviderReplayRestoresOriginalDataBeforeRetry(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	compose := filepath.Join(dir, "core.yaml")
	if err := os.WriteFile(compose, []byte("services:\n  openbao-member-1:\n    image: bao:2.6.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	delta := Delta{Installed: Realization{Kind: Secrets, Installation: "core", Scope: "shared", Instance: "openbao-member-1", Owner: "baseharbor", Image: "bao:2.6.0", Version: "2.6.0", Digest: digestA}, Desired: Desired{Kind: Secrets, Image: "bao:2.7.0", Version: "2.7.0", Digest: digestB}, Classification: BackupRequired}
	rt := &snapshotRuntime{data: []byte("openbao-before-upgrade")}
	ops := &mockNativeOps{failPinned: true}
	assets := map[string]NativeProviderAssets{JournalKey(delta): {
		Recovery: VolumeRecovery{Runtime: rt, Directory: filepath.Join(dir, "recovery"), Project: "p", Volume: "pg-backed-bao", VerifyQuiesced: func(context.Context, string, string) error { return nil }},
		Compose:  ComposeCheckpoint{Path: compose, Directory: filepath.Join(dir, "checkpoints")},
	}}
	plan := Plan{Release: "0.4.24", Deltas: []Delta{delta}}
	journal := filepath.Join(dir, "journal.json")
	if err := RunNativeProviderUpdates(context.Background(), plan, journal, ops, assets); err == nil {
		t.Fatal("failed provider restart reported success")
	}
	stateAfterRollback, err := LoadJournal(journal, "0.4.24")
	if err != nil {
		t.Fatal(err)
	}
	if stateAfterRollback.Steps[JournalKey(delta)] != "recovered" || rt.restores != 1 || ops.original != 1 {
		t.Fatalf("automatic rollback not committed: %+v; restores=%d ops=%+v", stateAfterRollback, rt.restores, ops)
	}
	ops.failPinned = false
	if err := RunNativeProviderUpdates(context.Background(), plan, journal, ops, assets); err != nil {
		t.Fatal(err)
	}
	if rt.exports != 1 || rt.restores != 1 || ops.original != 1 || ops.pinned != 2 {
		t.Fatalf("unsafe recovery chain exports=%d restores=%d ops=%+v", rt.exports, rt.restores, ops)
	}
	state, err := LoadJournal(journal, "0.4.24")
	if err != nil {
		t.Fatal(err)
	}
	if state.Steps[JournalKey(delta)] != "verified" {
		t.Fatalf("resumed provider not verified: %+v", state)
	}
}
