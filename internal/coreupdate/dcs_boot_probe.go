package coreupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	etcdbackup "github.com/mcpdev80/baseharbor/internal/corebackup/etcd"
)

// EtcdBootProbe requires a separately booted isolated recovery cluster.
// The snapshot itself is not evidence that etcd can form quorum.
type EtcdBootProbe struct {
	Binary           string
	Endpoints        []string
	TLS              etcdbackup.TLSFiles
	VerifyPatroniDCS func(context.Context) error
}

func (p EtcdBootProbe) Verify(ctx context.Context, id etcdbackup.Identity, snapshot etcdbackup.SnapshotInfo) error {
	if p.VerifyPatroniDCS == nil || len(p.Endpoints) != 3 || id.Cluster == "" || snapshot.ClusterID != id.Cluster || snapshot.Revision <= 0 {
		return errors.New("UNSUPPORTED: isolated etcd three-member boot and Patroni DCS proof required")
	}
	if !filepath.IsAbs(p.Binary) {
		return errors.New("trusted etcdctl absolute path required")
	}
	st, err := os.Lstat(p.Binary)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0022 != 0 {
		return errors.New("untrusted etcdctl executable")
	}
	if _, err := p.TLS.Config(); err != nil {
		return fmt.Errorf("etcd mTLS trust unavailable: %w", err)
	}
	members := map[string]bool{}
	leaders := 0
	memberIDs := map[string]bool{}
	var expectedLeader string
	var recoveredClusterID string
	for _, endpoint := range p.Endpoints {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return errors.New("foreign or invalid isolated etcd endpoint")
		}
		if members[endpoint] {
			return errors.New("duplicate etcd recovery endpoint")
		}
		members[endpoint] = true
		cmd := exec.CommandContext(ctx, p.Binary, "--endpoints="+endpoint, "endpoint", "status", "--write-out=json")
		cmd.Env = []string{"PATH=/usr/bin:/bin", "ETCDCTL_API=3", "ETCDCTL_CACERT=" + p.TLS.CA, "ETCDCTL_CERT=" + p.TLS.Cert, "ETCDCTL_KEY=" + p.TLS.Key}
		out, err := cmd.Output()
		if err != nil || len(out) > 32768 {
			return errors.New("recovered etcd endpoint status unavailable")
		}
		var status []struct {
			Endpoint string `json:"Endpoint"`
			Status   struct {
				Header struct {
					ClusterID json.Number `json:"cluster_id"`
					Revision  json.Number `json:"revision"`
					MemberID  json.Number `json:"member_id"`
				} `json:"header"`
				Leader   json.Number `json:"leader"`
				IsLeader bool        `json:"isLeader"`
				Version  string      `json:"version"`
			} `json:"Status"`
		}
		dec := json.NewDecoder(strings.NewReader(string(out)))
		dec.UseNumber()
		if err := dec.Decode(&status); err != nil || len(status) != 1 || status[0].Endpoint != endpoint {
			return errors.New("invalid authenticated etcd endpoint status")
		}
		s := status[0].Status
		revision, err := s.Header.Revision.Int64()
		if err != nil || revision < snapshot.Revision || s.Version != snapshot.Version || s.Leader.String() == "" || s.Leader.String() == "0" {
			return errors.New("etcd recovery version, leader or revision mismatch")
		}
		// etcdutl snapshot restore creates a NEW etcd cluster and member IDs.
		// Comparing the recovered cluster ID to the snapshot's old cluster ID
		// would reject every correctly isolated restoration.
		if err := validateRecoveredEtcdClusterID(id.Cluster, s.Header.ClusterID.String(), &recoveredClusterID); err != nil {
			return err
		}
		if err := validateRecoveredEtcdMemberID(s.Header.MemberID.String(), memberIDs); err != nil {
			return err
		}
		if expectedLeader == "" {
			expectedLeader = s.Leader.String()
		}
		if expectedLeader != s.Leader.String() {
			return errors.New("etcd recovered cluster has no common leader")
		}
		if s.IsLeader {
			if s.Header.MemberID.String() != s.Leader.String() {
				return errors.New("etcd self-reported leader differs from authenticated member identity")
			}
			leaders++
		}
	}
	if !memberIDs[expectedLeader] {
		return errors.New("etcd advertised leader is not among restored members")
	}
	if leaders != 1 {
		return errors.New("isolated etcd cluster must have exactly one leader and three healthy endpoints")
	}
	if err := p.VerifyPatroniDCS(ctx); err != nil {
		return fmt.Errorf("recovered Patroni DCS state unverified: %w", err)
	}
	return nil
}

func validateRecoveredEtcdClusterID(original, recovered string, expected *string) error {
	id, err := strconv.ParseUint(recovered, 10, 64)
	if err != nil || id == 0 || recovered == original {
		return errors.New("recovered etcd cluster identity is missing or was not rotated")
	}
	if expected == nil {
		return errors.New("recovered etcd cluster identity has no receipt destination")
	}
	if *expected == "" {
		*expected = recovered
	} else if *expected != recovered {
		return errors.New("isolated etcd endpoints disagree on recovered cluster identity")
	}
	return nil
}

func validateRecoveredEtcdMemberID(raw string, seen map[string]bool) error {
	memberID, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || memberID == 0 || seen[raw] {
		return errors.New("etcd member identity is missing or duplicated")
	}
	seen[raw] = true
	return nil
}
