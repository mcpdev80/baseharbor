package coreupdate

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	etcdbackup "github.com/mcpdev80/baseharbor/internal/corebackup/etcd"
)

type bridgeSnapshotSource struct{ calls int }

func (s *bridgeSnapshotSource) Snapshot(_ context.Context, w io.Writer) (etcdbackup.SnapshotInfo, error) {
	s.calls++
	_, err := io.WriteString(w, "isolated-etcd-snapshot-data")
	return etcdbackup.SnapshotInfo{ClusterID: "cluster-123", Version: "3.6.0", Revision: 42}, err
}

type bridgeIsolatedRestorer struct{ calls int }

func (r *bridgeIsolatedRestorer) RestoreIsolated(_ context.Context, _, dest string, _ etcdbackup.Identity, _ etcdbackup.SnapshotInfo) error {
	r.calls++
	if err := os.MkdirAll(filepath.Join(dest, "member", "snap"), 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, "member", "snap", "db"), []byte("restored"), 0600)
}
func (r *bridgeIsolatedRestorer) VerifyIsolated(_ context.Context, dest string, _ etcdbackup.Identity, _ etcdbackup.SnapshotInfo) error {
	data, err := os.ReadFile(filepath.Join(dest, "member", "snap", "db"))
	if err != nil {
		return err
	}
	if string(data) != "restored" {
		return errors.New("invalid isolated restore")
	}
	return nil
}
func TestEtcdDCSBridgeRestoresOnlyIsolatedOnce(t *testing.T) {
	dir := t.TempDir()
	source := &bridgeSnapshotSource{}
	restorer := &bridgeIsolatedRestorer{}
	bridge := &EtcdDCSBridge{
		Store:    etcdbackup.Store{Directory: filepath.Join(dir, "snapshot"), Identity: etcdbackup.Identity{Core: "core-123", Target: "target-123", Cluster: "cluster-123"}, Client: source},
		Restorer: restorer, RecoveryDirectory: filepath.Join(dir, "isolated"), Release: "0.4.24",
		VerifyRecoveredCluster: func(_ context.Context, id etcdbackup.Identity, info etcdbackup.SnapshotInfo) error {
			if id.Cluster != "cluster-123" || info.Revision != 42 {
				return errors.New("invalid boot attestation")
			}
			return nil
		},
	}
	ctx := context.Background()
	ev, err := bridge.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Target != "target-123" || len(ev.SHA256) != 64 {
		t.Fatalf("invalid DCS evidence: %+v", ev)
	}
	for i := 0; i < 2; i++ {
		if err := VerifyDCSEvidence(ctx, bridge, ev, "core-123", "target-123", "cluster-123", "0.4.24"); err != nil {
			t.Fatal(err)
		}
	}
	if restorer.calls != 1 || source.calls != 1 {
		t.Fatalf("snapshot or isolated restore repeated: source=%d restore=%d", source.calls, restorer.calls)
	}
	ev.Target = "foreign-target"
	if err := bridge.Validate(ctx, ev); !errors.Is(err, ErrDCSInvalidEvidence) {
		t.Fatalf("foreign snapshot accepted: %v", err)
	}
	if err := bridge.Restore(ctx, bridge.evidence(etcdbackup.Metadata{Identity: bridge.Store.Identity, SHA256: strings.Repeat("0", 64)})); err == nil {
		t.Fatal("foreign snapshot restore accepted")
	}
}
