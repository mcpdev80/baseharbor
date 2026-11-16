package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/dcspki"
)

// This is an opt-in real three-member process gate, not a RuntimeProvider mock.
// It uses only loopback ports and its private temp directory. No shared Docker
// daemon, existing Core, system trust store or systemd service is touched.
func TestRealEtcdMTLSSnapshotRestoreQuorum(t *testing.T) {
	dir := os.Getenv("BASEHARBOR_ETCD_TEST_BIN_DIR")
	if dir == "" {
		t.Skip("set BASEHARBOR_ETCD_TEST_BIN_DIR to trusted etcd/etcdctl/etcdutl binaries")
	}
	for _, name := range []string{"etcd", "etcdctl", "etcdutl"} {
		if err := trustedBinary(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	pki, err := dcspki.Ensure(filepath.Join(root, "pki"), []string{"localhost"})
	if err != nil {
		t.Fatal(err)
	}
	var clients, peers, names []string
	for i := 0; i < 3; i++ {
		names = append(names, fmt.Sprintf("member-%d", i+1))
		clients = append(clients, testLoopbackURL(t))
		peers = append(peers, testLoopbackURL(t))
	}
	var topology []string
	for i, name := range names {
		topology = append(topology, name+"="+peers[i])
	}
	initial := strings.Join(topology, ",")
	tlsArgs := []string{"--cacert=" + pki.CA, "--cert=" + pki.ClientCert, "--key=" + pki.ClientKey, "--dial-timeout=1s", "--command-timeout=2s"}
	ctl := func(endpoint string, args ...string) ([]byte, error) {
		base := append([]string{"--endpoints=" + endpoint}, tlsArgs...)
		cmd := exec.CommandContext(ctx, filepath.Join(dir, "etcdctl"), append(base, args...)...)
		return cmd.Output()
	}
	boot := func(directories []string, token string) func() {
		var processes []*exec.Cmd
		stopped := false
		stop := func() {
			if stopped {
				return
			}
			stopped = true
			for _, cmd := range processes {
				_ = cmd.Process.Kill()
			}
			for _, cmd := range processes {
				_ = cmd.Wait()
			}
		}
		t.Cleanup(stop)
		for i, name := range names {
			log, err := os.Create(filepath.Join(root, name+"-"+token+".log"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = log.Close() })
			args := []string{"--name=" + name, "--data-dir=" + directories[i], "--listen-client-urls=" + clients[i], "--advertise-client-urls=" + clients[i], "--listen-peer-urls=" + peers[i], "--initial-advertise-peer-urls=" + peers[i], "--initial-cluster=" + initial, "--initial-cluster-token=" + token, "--cert-file=" + pki.ServerCert, "--key-file=" + pki.ServerKey, "--client-cert-auth=true", "--trusted-ca-file=" + pki.CA, "--peer-cert-file=" + pki.ServerCert, "--peer-key-file=" + pki.ServerKey, "--peer-client-cert-auth=true", "--peer-trusted-ca-file=" + pki.CA, "--logger=zap", "--log-level=error"}
			cmd := exec.CommandContext(ctx, filepath.Join(dir, "etcd"), args...)
			cmd.Stdout = log
			cmd.Stderr = log
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			processes = append(processes, cmd)
		}
		for _, endpoint := range clients {
			for {
				if _, err := ctl(endpoint, "endpoint", "health"); err == nil {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatalf("real etcd endpoint %s not healthy: %v", endpoint, ctx.Err())
				case <-time.After(100 * time.Millisecond):
				}
			}
		}
		return stop
	}
	status := func(endpoint string) realEtcdStatus {
		out, err := ctl(endpoint, "endpoint", "status", "--write-out=json")
		if err != nil {
			t.Fatal(err)
		}
		var rows []struct {
			Status realEtcdStatus `json:"Status"`
		}
		if err := json.Unmarshal(out, &rows); err != nil || len(rows) != 1 {
			t.Fatalf("bad real status: %s, %v", out, err)
		}
		return rows[0].Status
	}
	checkQuorum := func(oldCluster string, minRevision int64, oldMembers map[uint64]bool) string {
		cluster := uint64(0)
		leader := uint64(0)
		members := map[uint64]bool{}
		leaders := 0
		for _, endpoint := range clients {
			s := status(endpoint)
			if s.Header.ClusterID == 0 || s.Header.MemberID == 0 || s.Leader == 0 || s.Header.Revision < minRevision || members[s.Header.MemberID] {
				t.Fatal("invalid real restored quorum")
			}
			if oldMembers[s.Header.MemberID] {
				t.Fatal("restore reused an old member identity")
			}
			if cluster == 0 {
				cluster = s.Header.ClusterID
				leader = s.Leader
			} else if cluster != s.Header.ClusterID || leader != s.Leader {
				t.Fatal("real endpoints disagree on cluster/leader")
			}
			members[s.Header.MemberID] = true
			if s.Header.MemberID == s.Leader {
				leaders++
				if s.Header.MemberID != leader {
					t.Fatal("leader mismatch")
				}
			}
		}
		if leaders != 1 || !members[leader] {
			t.Fatal("quorum lacks exactly one leader")
		}
		id := strconv.FormatUint(cluster, 10)
		if oldCluster != "" && oldCluster == id {
			t.Fatal("restore reused live cluster identity")
		}
		return id
	}
	liveDirs := make([]string, 3)
	for i, name := range names {
		liveDirs[i] = filepath.Join(root, "live-"+name)
	}
	stopLive := boot(liveDirs, "baseharbor-live-test")
	if _, err := ctl(clients[0], "put", "/service/baseharbor-test/initialize", "owned-postgresql-system-id"); err != nil {
		t.Fatal(err)
	}
	oldCluster := checkQuorum("", 1, nil)
	oldMembers := map[uint64]bool{}
	for _, endpoint := range clients {
		oldMembers[status(endpoint).Header.MemberID] = true
	}
	id := Identity{Core: "core-integration", Target: "target-integration", Cluster: oldCluster}
	scratch := filepath.Join(root, "scratch")
	if err := os.Mkdir(scratch, 0700); err != nil {
		t.Fatal(err)
	}
	source := EtcdctlSource{Etcdctl: filepath.Join(dir, "etcdctl"), Etcdutl: filepath.Join(dir, "etcdutl"), Endpoints: clients, Identity: id, TLS: TLSFiles{CA: pki.CA, Cert: pki.ClientCert, Key: pki.ClientKey}, ScratchDir: scratch}
	// Force a real write between attestation and snapshot. This must advance,
	// rather than invalidate, the native maintenance snapshot revision.
	source.Attest = func(ctx context.Context) (SnapshotInfo, error) {
		before, err := source.attestEndpointStatus(ctx)
		if err != nil {
			return SnapshotInfo{}, err
		}
		_, err = ctl(clients[0], "put", "/snapshot-after-attestation", "retained")
		return before, err
	}
	store := Store{Directory: filepath.Join(root, "backup"), Identity: id, Client: source}
	meta, err := store.CreateSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Info.Version != "3.7.2" {
		t.Fatalf("gate requires release-pinned etcd 3.7.2, got %s", meta.Info.Version)
	}
	// mTLS endpoints reject a caller with no client identity.
	unauthCtx, unauthCancel := context.WithTimeout(ctx, 2*time.Second)
	unauth := exec.CommandContext(unauthCtx, filepath.Join(dir, "etcdctl"), "--endpoints="+clients[0], "--cacert="+pki.CA, "endpoint", "health")
	if err := unauth.Run(); err == nil {
		t.Fatal("etcd accepted unauthenticated client")
	}
	unauthCancel()
	stopLive()
	restoreDirs := make([]string, 3)
	for i, name := range names {
		restoreDirs[i] = filepath.Join(root, "restored-"+name)
		restorer := IsolatedEtcdutl{Binary: filepath.Join(dir, "etcdutl"), MemberName: name, InitialCluster: initial, InitialAdvertisePeerURLs: peers[i], ExpectedMemberNames: names}
		plan, err := store.PrepareRestore(ctx, restoreDirs[i])
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Restore(ctx, plan, restorer); err != nil {
			t.Fatal(err)
		}
		if err := store.VerifyRecovery(ctx, plan, restorer); err != nil {
			t.Fatal(err)
		}
	}
	token, err := RecoveryClusterToken(ctx, filepath.Join(store.Directory, "etcd.snapshot"), id)
	if err != nil {
		t.Fatal(err)
	}
	stopRestored := boot(restoreDirs, token)
	defer stopRestored()
	newCluster := checkQuorum(oldCluster, meta.Info.Revision, oldMembers)
	for _, endpoint := range clients {
		out, err := ctl(endpoint, "get", "/snapshot-after-attestation", "--print-value-only")
		if err != nil || strings.TrimSpace(string(out)) != "retained" {
			t.Fatalf("restored data missing: %q %v", out, err)
		}
	}
	t.Logf("REAL_ETCD_MTLS_RESTORE_PASS version=%s old_cluster=%s new_cluster=%s revision=%d members=3 leaders=1 uid=%d", meta.Info.Version, oldCluster, newCluster, meta.Info.Revision, os.Getuid())
}

type realEtcdStatus struct {
	Header struct {
		ClusterID uint64 `json:"cluster_id"`
		MemberID  uint64 `json:"member_id"`
		Revision  int64  `json:"revision"`
	} `json:"header"`
	Leader   uint64 `json:"leader"`
	IsLeader bool   `json:"isLeader"`
}

func testLoopbackURL(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	return "https://" + addr
}
