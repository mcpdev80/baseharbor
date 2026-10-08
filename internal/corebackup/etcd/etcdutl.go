package etcd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// IsolatedEtcdutl restores a validated snapshot with an independent trusted
// etcdutl binary. The existing cluster data directory is never touched.
type IsolatedEtcdutl struct {
	Binary                   string
	MemberName               string
	InitialCluster           string
	InitialAdvertisePeerURLs string
	ExpectedMemberNames      []string
}

func (e IsolatedEtcdutl) validate() error {
	if e.Binary == "" || e.MemberName == "" || e.InitialCluster == "" || e.InitialAdvertisePeerURLs == "" || len(e.ExpectedMemberNames) == 0 {
		return errors.New("isolated etcdutl requires explicit binary and new member topology")
	}
	st, err := os.Lstat(e.Binary)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0022 != 0 {
		return errors.New("etcdutl binary is not a trusted regular file")
	}
	seen := map[string]bool{}
	for _, name := range e.ExpectedMemberNames {
		if name == "" || strings.ContainsAny(name, " =,\n\r\t") || seen[name] {
			return errors.New("invalid or duplicate intended member")
		}
		seen[name] = true
	}
	if !seen[e.MemberName] {
		return errors.New("restore member is not in new cluster topology")
	}
	for _, part := range strings.Split(e.InitialCluster, ",") {
		x := strings.SplitN(part, "=", 2)
		if len(x) != 2 || !seen[x[0]] || !strings.HasPrefix(x[1], "https://") {
			return errors.New("initial cluster includes unexpected member or non-TLS peer URL")
		}
		delete(seen, x[0])
	}
	if len(seen) != 0 {
		return errors.New("initial cluster does not contain all expected members")
	}
	for _, url := range strings.Split(e.InitialAdvertisePeerURLs, ",") {
		if !strings.HasPrefix(url, "https://") {
			return errors.New("advertised peer URL must use TLS")
		}
	}
	return nil
}
func (e IsolatedEtcdutl) RestoreIsolated(ctx context.Context, archive, destination string, identity Identity, info SnapshotInfo) error {
	if err := identity.Validate(); err != nil {
		return err
	}
	if err := e.validate(); err != nil {
		return err
	}
	if info.ClusterID != identity.Cluster || info.Revision <= 0 || info.Version == "" {
		return errors.New("invalid snapshot identity for restore")
	}
	st, err := os.Lstat(archive)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm() != 0600 {
		return errors.New("unsafe snapshot input")
	}
	if _, err := os.Lstat(destination); err == nil {
		return errors.New("restore target exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(destination)
	if err := safeDirectory(parent); err != nil {
		return err
	}
	// No sh, no credentials, no live etcd data directory. Restore creates an
	// isolated target that must be tested before any cutover.
	args := []string{"snapshot", "restore", archive, "--data-dir", destination, "--name", e.MemberName, "--initial-cluster", e.InitialCluster, "--initial-advertise-peer-urls", e.InitialAdvertisePeerURLs}
	cmd := exec.CommandContext(ctx, e.Binary, args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("isolated etcdutl restore unsuccessful: %w", err)
	}
	return nil
}
func (e IsolatedEtcdutl) VerifyIsolated(ctx context.Context, destination string, identity Identity, info SnapshotInfo) error {
	if err := identity.Validate(); err != nil {
		return err
	}
	if err := e.validate(); err != nil {
		return err
	}
	if info.ClusterID != identity.Cluster || info.Revision <= 0 {
		return errors.New("invalid recovery identity")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	st, err := os.Lstat(destination)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return errors.New("isolated restore directory is not owner-only")
	}
	// This checks disk existence only. Live member startup, DCS quorum and
	// Patroni recovery require integration with the Session 1A orchestrator.
	for _, p := range []string{"member", "member/snap", "member/snap/db"} {
		st, err := os.Lstat(filepath.Join(destination, p))
		if err != nil {
			return err
		}
		if p == "member/snap/db" && !st.Mode().IsRegular() {
			return errors.New("restored etcd database is not regular")
		}
		if p != "member/snap/db" && !st.IsDir() {
			return errors.New("restored etcd directory structure invalid")
		}
	}
	return nil
}
