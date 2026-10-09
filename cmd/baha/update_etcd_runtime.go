package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	etcdbackup "github.com/mcpdev80/baseharbor/internal/corebackup/etcd"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const coreEtcdRecoveryService = "postgres-etcd-recovery"

type runtimeEtcdTools struct {
	Runtime         bhruntime.RuntimeProvider
	Files           bhruntime.Files
	Service         string
	Endpoints       []string
	ExpectedCluster string
	ScratchDir      string
	ContainerCA     string
	ContainerCert   string
	ContainerKey    string
	Members         []string
	InitialCluster  string
	Identity        etcdRecoveryIdentity
}

func (t runtimeEtcdTools) validate() error {
	if t.Runtime == nil || !t.Files.HA || t.Files.Project == "" || t.Files.Compose == "" || t.Files.Env == "" ||
		t.Service == "" || len(t.Endpoints) != 3 || t.ScratchDir == "" ||
		t.ContainerCA == "" || t.ContainerCert == "" || t.ContainerKey == "" || len(t.Members) != 3 ||
		t.InitialCluster == "" {
		return errors.New("runtime etcd recovery requires explicit HA runtime, mTLS identity and three-member topology")
	}
	seen := map[string]bool{}
	for _, member := range t.Members {
		if member == "" || seen[member] || strings.ContainsAny(member, " =,/\\\n\r\t") {
			return errors.New("invalid or duplicate etcd recovery member")
		}
		seen[member] = true
	}
	for _, endpoint := range t.Endpoints {
		if !strings.HasPrefix(endpoint, "https://") {
			return errors.New("runtime etcd recovery endpoint must use HTTPS")
		}
	}
	return nil
}

func (t runtimeEtcdTools) run(ctx context.Context, binary string, stdout, stderr io.Writer, binds []string, args ...string) error {
	if err := t.validate(); err != nil {
		return err
	}
	environment, err := bhruntime.RuntimeEnvironment(t.Files)
	if err != nil {
		return err
	}
	uidgid := strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())
	if t.Identity.User != "" {
		uidgid = t.Identity.User
	}
	runArgs := []string{"run", "--rm", "--no-deps", "--user", uidgid}
	for _, bind := range binds {
		if strings.TrimSpace(bind) == "" {
			return errors.New("empty etcd recovery bind mount")
		}
		runArgs = append(runArgs, "-v", bind)
	}
	runArgs = append(runArgs, "--entrypoint", binary, t.Service)
	runArgs = append(runArgs, args...)
	return t.Runtime.RunProjectFilesEnv(ctx, t.Files.Project, filepath.Dir(t.Files.Compose), environment,
		nil, stdout, stderr, []string{t.Files.Compose}, runArgs...)
}

func (t runtimeEtcdTools) tlsArgs() []string {
	return []string{
		"--cacert=" + t.ContainerCA,
		"--cert=" + t.ContainerCert,
		"--key=" + t.ContainerKey,
	}
}

type runtimeEtcdStatus struct {
	Endpoint  string
	ClusterID string
	MemberID  string
	LeaderID  string
	Revision  int64
	Version   string
	IsLeader  bool
}

func parseRuntimeEtcdStatus(data []byte) (runtimeEtcdStatus, error) {
	if len(data) == 0 || len(data) > 32768 {
		return runtimeEtcdStatus{}, errors.New("invalid etcd endpoint status length")
	}
	var rows []struct {
		Endpoint string `json:"Endpoint"`
		Status   struct {
			Header struct {
				ClusterID json.Number `json:"cluster_id"`
				MemberID  json.Number `json:"member_id"`
				Revision  json.Number `json:"revision"`
			} `json:"header"`
			Leader   json.Number `json:"leader"`
			Version  string      `json:"version"`
			IsLeader *bool       `json:"isLeader"`
		} `json:"Status"`
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	if err := dec.Decode(&rows); err != nil || len(rows) != 1 {
		return runtimeEtcdStatus{}, errors.New("invalid etcd endpoint status JSON")
	}
	revision, err := rows[0].Status.Header.Revision.Int64()
	if err != nil || revision <= 0 || rows[0].Status.Header.ClusterID == "" ||
		rows[0].Status.Header.MemberID == "" || rows[0].Status.Leader == "" ||
		strings.TrimSpace(rows[0].Status.Version) == "" {
		return runtimeEtcdStatus{}, errors.New("incomplete authenticated etcd endpoint status")
	}
	// etcd IDs are unsigned 64-bit values. Reject malformed or zero IDs,
	// including quoted nonnumeric JSON values, before trusting quorum status.
	ids := []json.Number{rows[0].Status.Header.ClusterID, rows[0].Status.Header.MemberID, rows[0].Status.Leader}
	for _, id := range ids {
		value, parseErr := strconv.ParseUint(id.String(), 10, 64)
		if parseErr != nil || value == 0 {
			return runtimeEtcdStatus{}, errors.New("invalid unsigned etcd cluster/member/leader identity")
		}
	}
	// The etcd v3 StatusResponse reports leader/member IDs, not an isLeader
	// field. Compute leadership from those authenticated IDs. If an optional
	// helper supplies the field, reject contradictory evidence.
	isLeader := rows[0].Status.Header.MemberID == rows[0].Status.Leader
	if rows[0].Status.IsLeader != nil && *rows[0].Status.IsLeader != isLeader {
		return runtimeEtcdStatus{}, errors.New("etcd leader flag contradicts authenticated member identity")
	}
	return runtimeEtcdStatus{
		Endpoint: rows[0].Endpoint, ClusterID: rows[0].Status.Header.ClusterID.String(),
		MemberID: rows[0].Status.Header.MemberID.String(), LeaderID: rows[0].Status.Leader.String(),
		Revision: revision, Version: rows[0].Status.Version, IsLeader: isLeader,
	}, nil
}

func (t runtimeEtcdTools) attest(ctx context.Context) (etcdbackup.SnapshotInfo, error) {
	if err := t.validate(); err != nil {
		return etcdbackup.SnapshotInfo{}, err
	}
	var result etcdbackup.SnapshotInfo
	statuses := make([]runtimeEtcdStatus, 0, len(t.Endpoints))
	for _, endpoint := range t.Endpoints {
		var out strings.Builder
		args := append([]string{"--endpoints=" + endpoint}, t.tlsArgs()...)
		args = append(args, "endpoint", "status", "--write-out=json")
		if err := t.run(ctx, "/usr/local/bin/etcdctl", &out, io.Discard, nil, args...); err != nil {
			return etcdbackup.SnapshotInfo{}, fmt.Errorf("authenticated etcd status %s: %w", endpoint, err)
		}
		status, err := parseRuntimeEtcdStatus([]byte(out.String()))
		if err != nil {
			return etcdbackup.SnapshotInfo{}, err
		}
		if status.Endpoint != endpoint || (t.ExpectedCluster != "" && status.ClusterID != t.ExpectedCluster) {
			return etcdbackup.SnapshotInfo{}, errors.New("foreign etcd endpoint or cluster identity")
		}
		if result.ClusterID != "" && (result.ClusterID != status.ClusterID || result.Version != status.Version) {
			return etcdbackup.SnapshotInfo{}, errors.New("etcd endpoints disagree on cluster identity or version")
		}
		statuses = append(statuses, status)
		if result.Revision == 0 || status.Revision < result.Revision {
			result = etcdbackup.SnapshotInfo{ClusterID: status.ClusterID, Version: status.Version, Revision: status.Revision}
		}
	}
	if err := verifyRuntimeEtcdQuorum(statuses); err != nil {
		return etcdbackup.SnapshotInfo{}, err
	}
	return result, nil
}

// verifyRuntimeEtcdQuorum refuses an authenticated but split-brain or
// duplicated endpoint inventory before a DCS recovery point can be captured.
func verifyRuntimeEtcdQuorum(statuses []runtimeEtcdStatus) error {
	if len(statuses) != 3 {
		return errors.New("etcd DCS requires three authenticated quorum members")
	}
	members := map[string]bool{}
	leader, cluster := "", ""
	leaders := 0
	for _, status := range statuses {
		if status.MemberID == "" || status.MemberID == "0" || status.ClusterID == "" || status.ClusterID == "0" ||
			status.LeaderID == "" || status.LeaderID == "0" || status.Revision <= 0 || members[status.MemberID] {
			return errors.New("etcd DCS quorum has invalid or duplicate member identity")
		}
		members[status.MemberID] = true
		if cluster == "" {
			cluster, leader = status.ClusterID, status.LeaderID
		} else if cluster != status.ClusterID || leader != status.LeaderID {
			return errors.New("etcd DCS quorum disagrees on cluster or leader")
		}
		if status.IsLeader {
			if status.MemberID != leader {
				return errors.New("etcd DCS leader identity is inconsistent")
			}
			leaders++
		}
	}
	if leaders != 1 || !members[leader] {
		return errors.New("etcd DCS quorum lacks exactly one owned leader")
	}
	return nil
}

type runtimeEtcdSnapshotStatus struct {
	Revision  int64  `json:"revision"`
	TotalKey  int64  `json:"totalKey"`
	TotalSize int64  `json:"totalSize"`
	Hash      uint64 `json:"hash"`
}

func parseRuntimeEtcdSnapshotStatus(data []byte) (runtimeEtcdSnapshotStatus, error) {
	if len(data) == 0 || len(data) > 8192 {
		return runtimeEtcdSnapshotStatus{}, errors.New("invalid etcdutl status length")
	}
	var status runtimeEtcdSnapshotStatus
	if err := json.Unmarshal(data, &status); err != nil || status.Revision <= 0 || status.TotalSize <= 0 || status.TotalKey <= 0 {
		return runtimeEtcdSnapshotStatus{}, errors.New("invalid etcdutl snapshot status")
	}
	return status, nil
}

func verifyRuntimeEtcdSnapshotRevision(attested, snapshot int64) error {
	if attested <= 0 || snapshot < attested {
		return errors.New("snapshot revision predates authenticated cluster revision")
	}
	return nil
}

func (t runtimeEtcdTools) Snapshot(ctx context.Context, dest io.Writer) (etcdbackup.SnapshotInfo, error) {
	if dest == nil {
		return etcdbackup.SnapshotInfo{}, errors.New("etcd snapshot destination required")
	}
	attested, err := t.attest(ctx)
	if err != nil {
		return etcdbackup.SnapshotInfo{}, err
	}
	if err := os.MkdirAll(t.ScratchDir, 0o700); err != nil {
		return etcdbackup.SnapshotInfo{}, err
	}
	if err := os.Chmod(t.ScratchDir, 0o700); err != nil {
		return etcdbackup.SnapshotInfo{}, err
	}
	scratch, err := os.MkdirTemp(t.ScratchDir, ".etcd-runtime-*")
	if err != nil {
		return etcdbackup.SnapshotInfo{}, err
	}
	defer os.RemoveAll(scratch)
	if err := os.Chmod(scratch, 0o700); err != nil {
		return etcdbackup.SnapshotInfo{}, err
	}
	archive := filepath.Join(scratch, "snapshot.db")
	bind := scratch + ":/recovery"
	args := append([]string{"--endpoints=" + t.Endpoints[0]}, t.tlsArgs()...)
	args = append(args, "snapshot", "save", "/recovery/snapshot.db")
	if err := t.run(ctx, "/usr/local/bin/etcdctl", io.Discard, io.Discard, []string{bind}, args...); err != nil {
		return etcdbackup.SnapshotInfo{}, fmt.Errorf("etcd maintenance snapshot failed: %w", err)
	}
	if err := os.Chmod(archive, 0o600); err != nil {
		return etcdbackup.SnapshotInfo{}, err
	}
	var statusOut strings.Builder
	if err := t.run(ctx, "/usr/local/bin/etcdutl", &statusOut, io.Discard, []string{bind},
		"snapshot", "status", "/recovery/snapshot.db", "--write-out=json"); err != nil {
		return etcdbackup.SnapshotInfo{}, fmt.Errorf("etcdutl snapshot status failed: %w", err)
	}
	status, err := parseRuntimeEtcdSnapshotStatus([]byte(statusOut.String()))
	if err != nil {
		return etcdbackup.SnapshotInfo{}, err
	}
	// A live cluster can advance between the quorum probe and the
	// consistent maintenance snapshot. A newer snapshot is expected;
	// an older one would miss state already observed during attestation.
	if err := verifyRuntimeEtcdSnapshotRevision(attested.Revision, status.Revision); err != nil {
		return etcdbackup.SnapshotInfo{}, err
	}
	f, err := os.Open(archive)
	if err != nil {
		return etcdbackup.SnapshotInfo{}, err
	}
	defer f.Close()
	if _, err := io.Copy(dest, f); err != nil {
		return etcdbackup.SnapshotInfo{}, err
	}
	return etcdbackup.SnapshotInfo{ClusterID: attested.ClusterID, Version: attested.Version, Revision: status.Revision}, nil
}

func (t runtimeEtcdTools) RestoreIsolated(ctx context.Context, archive, destination string, identity etcdbackup.Identity, info etcdbackup.SnapshotInfo) error {
	if err := t.validate(); err != nil {
		return err
	}
	if identity.Cluster != t.ExpectedCluster || info.ClusterID != identity.Cluster || info.Revision <= 0 {
		return errors.New("invalid runtime etcd restore identity")
	}
	st, err := os.Lstat(archive)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm() != 0o600 {
		return errors.New("unsafe etcd snapshot input")
	}
	if _, err := os.Lstat(destination); err == nil {
		return errors.New("isolated etcd restore destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(destination, 0o700); err != nil {
		return err
	}
	archiveDir := filepath.Dir(archive)
	archiveName := filepath.Base(archive)
	token, err := etcdbackup.RecoveryClusterToken(ctx, archive, identity)
	if err != nil {
		return err
	}
	binds := []string{archiveDir + ":/snapshot:ro", destination + ":/restore"}
	for _, member := range t.Members {
		args := []string{
			"snapshot", "restore", "/snapshot/" + archiveName,
			"--data-dir", "/restore/" + member,
			"--name", member,
			"--initial-cluster", t.InitialCluster,
			"--initial-cluster-token", token,
			"--initial-advertise-peer-urls", "https://" + member + ":2380",
		}
		if err := t.run(ctx, "/usr/local/bin/etcdutl", io.Discard, io.Discard, binds, args...); err != nil {
			return fmt.Errorf("isolated etcd restore member %s: %w", member, err)
		}
	}
	return nil
}

func (t runtimeEtcdTools) VerifyIsolated(ctx context.Context, destination string, identity etcdbackup.Identity, info etcdbackup.SnapshotInfo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := t.validate(); err != nil {
		return err
	}
	if identity.Cluster != t.ExpectedCluster || info.ClusterID != identity.Cluster || info.Revision <= 0 {
		return errors.New("invalid isolated etcd recovery identity")
	}
	root, err := os.Lstat(destination)
	if err != nil {
		return err
	}
	if !root.IsDir() || root.Mode().Perm()&0o077 != 0 {
		return errors.New("isolated etcd recovery directory is not owner-only")
	}
	for _, member := range t.Members {
		for _, suffix := range []string{"member", "member/snap", "member/snap/db"} {
			path := filepath.Join(destination, member, suffix)
			st, err := os.Lstat(path)
			if err != nil {
				return err
			}
			if suffix == "member/snap/db" {
				if !st.Mode().IsRegular() {
					return errors.New("restored etcd database is not regular")
				}
			} else if !st.IsDir() {
				return errors.New("restored etcd directory structure invalid")
			}
		}
	}
	return nil
}
