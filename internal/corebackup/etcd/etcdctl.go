package etcd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// EtcdctlSource invokes a separately installed trusted etcdctl and etcdutl,
// never a command inside the distroless etcd container. Credentials remain in
// protected files; only path locations are passed through the child environment.
type EtcdctlSource struct {
	Etcdctl    string
	Etcdutl    string
	Endpoints  []string
	Identity   Identity
	TLS        TLSFiles
	ScratchDir string
	Attest     func(context.Context) (SnapshotInfo, error)
}

// pinnedEnv builds the child environment so that the controlled etcd keys are
// never overridable by the parent environment. It copies os.Environ(), removes
// every key listed in pins, and appends the pin values last. Dropping the key
// from the inherited copy is mandatory: with Go's env-dedup (last value wins)
// an inherited ETCDCTL_ENDPOINTS/ETCDCTL_CACERT/... placed before the pin would
// silently win, making the pinned endpoint and TLS trust swappable by a parent
// process. The result is therefore independent of Go's dedup rule.
func pinnedEnv(pins ...string) []string {
	dropped := make(map[string]struct{}, len(pins))
	for _, p := range pins {
		if i := indexByte(p, '='); i >= 0 {
			dropped[p[:i]] = struct{}{}
		}
	}
	out := make([]string, 0, len(os.Environ())+len(pins))
	for _, kv := range os.Environ() {
		if i := indexByte(kv, '='); i >= 0 {
			if _, bad := dropped[kv[:i]]; bad {
				continue
			}
		}
		out = append(out, kv)
	}
	return append(out, pins...)
}

// indexByte returns the offset of the first c in s, or -1 when absent.
func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func trustedBinary(path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return errors.New("trusted helper must have an absolute path")
	}
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0022 != 0 {
		return errors.New("untrusted helper executable")
	}
	return nil
}
func (s EtcdctlSource) validate() error {
	if err := s.Identity.Validate(); err != nil {
		return err
	}
	if err := trustedBinary(s.Etcdctl); err != nil {
		return err
	}
	if err := trustedBinary(s.Etcdutl); err != nil {
		return err
	}
	if len(s.Endpoints) == 0 || len(s.Endpoints) > 9 {
		return errors.New("explicit owned etcd endpoints required")
	}
	for _, endpoint := range s.Endpoints {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return errors.New("etcd endpoint must be an explicit HTTPS authority")
		}
	}
	if _, err := s.TLS.Config(); err != nil {
		return fmt.Errorf("etcd client mTLS unavailable: %w", err)
	}
	return safeDirectory(s.ScratchDir)
}
func (s EtcdctlSource) Snapshot(ctx context.Context, dest io.Writer) (SnapshotInfo, error) {
	if dest == nil {
		return SnapshotInfo{}, errors.New("snapshot stream destination required")
	}
	if err := s.validate(); err != nil {
		return SnapshotInfo{}, err
	}
	attest := s.Attest
	if attest == nil {
		attest = s.attestEndpointStatus
	}
	attested, err := attest(ctx)
	if err != nil {
		return SnapshotInfo{}, err
	}
	if attested.ClusterID != s.Identity.Cluster || attested.Version == "" || attested.Revision <= 0 {
		return SnapshotInfo{}, errors.New("etcd cluster attestation mismatch")
	}
	scratch, err := os.MkdirTemp(s.ScratchDir, ".etcdctl-*")
	if err != nil {
		return SnapshotInfo{}, err
	}
	defer os.RemoveAll(scratch)
	if err := os.Chmod(scratch, 0700); err != nil {
		return SnapshotInfo{}, err
	}
	archive := filepath.Join(scratch, "snapshot.db")
	// Only the approved endpoints are used. No token/certificate bytes in argv.
	// Pin the approved etcdctl keys first, then inherit the parent environment
	// (os.Environ) so exported variables reach the child; the child is a direct
	// descendant of the baha CLI and must inherit it.
	cmd := exec.CommandContext(ctx, s.Etcdctl, "snapshot", "save", archive)
	// The child is a direct descendant of the baha CLI and must inherit it, but
	// the controlled etcd keys are pinned via pinnedEnv, which drops any inherited
	// copy of those keys first so the pin is never overridden by the parent env.
	cmd.Env = pinnedEnv(
		"PATH=/usr/bin:/bin", "ETCDCTL_API=3",
		// Maintenance.Snapshot streams from one member, after all configured
		// endpoints have attested the owning cluster.
		"ETCDCTL_ENDPOINTS="+s.Endpoints[0],
		"ETCDCTL_CACERT="+s.TLS.CA,
		"ETCDCTL_CERT="+s.TLS.Cert,
		"ETCDCTL_KEY="+s.TLS.Key,
	)
	if err := cmd.Run(); err != nil {
		return SnapshotInfo{}, errors.New("etcd maintenance snapshot failed")
	}
	if err := os.Chmod(archive, 0600); err != nil {
		return SnapshotInfo{}, err
	}
	if err := safeRegular(archive); err != nil {
		return SnapshotInfo{}, err
	}
	// Isolation means never executing inside the running etcd container; the
	// child still inherits the parent env minus the pinned keys.
	verify := exec.CommandContext(ctx, s.Etcdutl, "snapshot", "status", archive, "--write-out=json")
	verify.Env = pinnedEnv("PATH=/usr/bin:/bin")
	statusJSON, err := verify.Output()
	if err != nil {
		return SnapshotInfo{}, errors.New("etcdutl snapshot status integrity check failed")
	}
	var status struct {
		Revision  int64  `json:"revision"`
		TotalKey  int64  `json:"totalKey"`
		TotalSize int64  `json:"totalSize"`
		Hash      uint64 `json:"hash"`
	}
	if len(statusJSON) > 8192 || json.Unmarshal(statusJSON, &status) != nil || status.Revision <= 0 || status.TotalSize <= 0 {
		return SnapshotInfo{}, errors.New("invalid etcdutl snapshot status")
	}
	f, err := os.Open(archive)
	if err != nil {
		return SnapshotInfo{}, err
	}
	defer f.Close()
	if _, err := io.Copy(dest, &contextReader{ctx: ctx, Reader: f}); err != nil {
		return SnapshotInfo{}, err
	}
	// The owning Core must supply attested cluster ID and etcd version from
	// authenticated etcd status. Snapshot bytes alone cannot attest membership.
	if status.Revision < attested.Revision {
		return SnapshotInfo{}, errors.New("snapshot revision predates authenticated cluster revision")
	}
	return SnapshotInfo{ClusterID: attested.ClusterID, Version: attested.Version, Revision: status.Revision}, nil
}

type endpointStatus struct {
	Header struct {
		ClusterID json.Number `json:"cluster_id"`
		Revision  json.Number `json:"revision"`
	} `json:"header"`
	Version string `json:"version"`
}

func (s EtcdctlSource) attestEndpointStatus(ctx context.Context) (SnapshotInfo, error) {
	var result SnapshotInfo
	for _, endpoint := range s.Endpoints {
		cmd := exec.CommandContext(ctx, s.Etcdctl, "--endpoints="+endpoint, "endpoint", "status", "--write-out=json")
		// Pin PATH, API and TLS trust via pinnedEnv so a parent ETCDCTL_* key
		// can never override them.
		cmd.Env = pinnedEnv(
			"PATH=/usr/bin:/bin", "ETCDCTL_API=3",
			"ETCDCTL_CACERT="+s.TLS.CA,
			"ETCDCTL_CERT="+s.TLS.Cert,
			"ETCDCTL_KEY="+s.TLS.Key,
		)
		output, err := cmd.Output()
		if err != nil || len(output) > 32768 {
			return SnapshotInfo{}, errors.New("etcd endpoint status authentication failed")
		}
		var statuses []struct {
			Endpoint string         `json:"Endpoint"`
			Status   endpointStatus `json:"Status"`
		}
		decoder := json.NewDecoder(strings.NewReader(string(output)))
		decoder.UseNumber()
		if err := decoder.Decode(&statuses); err != nil || len(statuses) != 1 || statuses[0].Endpoint != endpoint {
			return SnapshotInfo{}, errors.New("invalid authenticated etcd endpoint status")
		}
		st := statuses[0].Status
		revision, err := st.Header.Revision.Int64()
		if err != nil || revision <= 0 || st.Version == "" || st.Header.ClusterID == "" {
			return SnapshotInfo{}, errors.New("incomplete etcd cluster attestation")
		}
		// etcdctl emits uint64 cluster IDs as JSON integers. Compare the decimal
		// representation directly to the caller's pinned cluster identity.
		current := SnapshotInfo{ClusterID: st.Header.ClusterID.String(), Version: st.Version, Revision: revision}
		if current.ClusterID != s.Identity.Cluster {
			return SnapshotInfo{}, errors.New("foreign etcd cluster detected")
		}
		if result.ClusterID != "" && (result.ClusterID != current.ClusterID || result.Version != current.Version) {
			return SnapshotInfo{}, errors.New("inconsistent etcd endpoint identity or version")
		}
		if result.Revision == 0 || current.Revision < result.Revision {
			result = current
		}
	}
	return result, nil
}
