package etcd

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Identity struct {
	Core    string `json:"core"`
	Target  string `json:"target"`
	Cluster string `json:"cluster"`
}

func (i Identity) Validate() error {
	for _, s := range []string{i.Core, i.Target, i.Cluster} {
		if len(s) < 3 || len(s) > 128 || strings.ContainsAny(s, "/\\:\n\r\t ") || s == "." || s == ".." {
			return errors.New("invalid Core/target/cluster identity")
		}
	}
	return nil
}

type Endpoint struct {
	Address    string
	ServerName string
}
type TLSFiles struct{ CA, Cert, Key string }

func (t TLSFiles) Config() (*tls.Config, error) {
	if t.CA == "" || t.Cert == "" || t.Key == "" {
		return nil, errors.New("mTLS CA/cert/key required")
	}
	ca, err := os.ReadFile(t.CA)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, errors.New("invalid etcd CA")
	}
	cert, err := tls.LoadX509KeyPair(t.Cert, t.Key)
	if err != nil {
		return nil, errors.New("invalid etcd client identity")
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool, Certificates: []tls.Certificate{cert}}, nil
}

type Source interface {
	// Snapshot streams etcd v3 Maintenance.Snapshot (gRPC), never a container shell.
	Snapshot(context.Context, io.Writer) (SnapshotInfo, error)
}
type SnapshotInfo struct {
	ClusterID string `json:"cluster_id"`
	Version   string `json:"version"`
	Revision  int64  `json:"revision"`
}
type Metadata struct {
	Format   int          `json:"format"`
	Identity Identity     `json:"identity"`
	Info     SnapshotInfo `json:"info"`
	Bytes    int64        `json:"bytes"`
	SHA256   string       `json:"sha256"`
	Created  time.Time    `json:"created"`
}
type Store struct {
	Directory string
	Identity  Identity
	Client    Source
}

func (s Store) paths() (string, string, error) {
	if err := s.Identity.Validate(); err != nil {
		return "", "", err
	}
	if s.Directory == "" {
		return "", "", errors.New("backup directory required")
	}
	return filepath.Join(s.Directory, "etcd.snapshot"), filepath.Join(s.Directory, "etcd.metadata.json"), nil
}
func safeDirectory(dir string) error {
	if err := rejectSymlinkAncestors(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	st, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return errors.New("backup directory must be owner-only and not a symlink")
	}
	return nil
}
func safeRegular(path string) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || st.Size() == 0 {
		return errors.New("unsafe backup artifact")
	}
	return nil
}
func writeExclusive(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}
func (s Store) CreateSnapshot(ctx context.Context) (Metadata, error) {
	archive, meta, err := s.paths()
	if err != nil {
		return Metadata{}, err
	}
	if s.Client == nil {
		return Metadata{}, errors.New("etcd snapshot client required")
	}
	if err := safeDirectory(s.Directory); err != nil {
		return Metadata{}, err
	}
	// Never replace a prior completed or interrupted artifact implicitly.
	if _, err := os.Lstat(archive); err == nil {
		return s.VerifySnapshot(ctx)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Metadata{}, err
	}
	if _, err := os.Lstat(meta); err == nil {
		return Metadata{}, errors.New("orphaned snapshot metadata must be investigated")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Metadata{}, err
	}
	tmp, err := os.CreateTemp(s.Directory, ".etcd-incomplete-*")
	if err != nil {
		return Metadata{}, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if err := tmp.Chmod(0600); err != nil {
		return Metadata{}, err
	}
	h := sha256.New()
	written := &countWriter{Writer: io.MultiWriter(tmp, h)}
	info, err := s.Client.Snapshot(ctx, written)
	if err != nil {
		return Metadata{}, fmt.Errorf("etcd snapshot stream failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Metadata{}, err
	}
	if info.ClusterID != s.Identity.Cluster || info.Version == "" || info.Revision <= 0 || written.Count == 0 {
		return Metadata{}, errors.New("snapshot cluster/version/revision or length invalid")
	}
	if err := tmp.Sync(); err != nil {
		return Metadata{}, err
	}
	m := Metadata{Format: 1, Identity: s.Identity, Info: info, Bytes: written.Count, SHA256: hex.EncodeToString(h.Sum(nil)), Created: time.Now().UTC()}
	encoded, err := json.Marshal(m)
	if err != nil {
		return Metadata{}, err
	}
	// Publishing snapshot first means interruption remains fail-closed, never mistaken for success.
	if err := os.Link(tmp.Name(), archive); err != nil {
		return Metadata{}, err
	}
	if err := writeExclusive(meta, encoded); err != nil {
		return Metadata{}, err
	}
	if err := syncDir(s.Directory); err != nil {
		return Metadata{}, err
	}
	return m, nil
}

type countWriter struct {
	io.Writer
	Count int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, e := c.Writer.Write(p)
	c.Count += int64(n)
	return n, e
}
func syncDir(dir string) error {
	f, e := os.Open(dir)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func (s Store) VerifySnapshot(ctx context.Context) (Metadata, error) {
	archive, meta, err := s.paths()
	if err != nil {
		return Metadata{}, err
	}
	if err := safeDirectory(s.Directory); err != nil {
		return Metadata{}, err
	}
	if err := safeRegular(archive); err != nil {
		return Metadata{}, err
	}
	if err := safeRegular(meta); err != nil {
		return Metadata{}, err
	}
	data, err := os.ReadFile(meta)
	if err != nil {
		return Metadata{}, err
	}
	var m Metadata
	if len(data) > 16384 || json.Unmarshal(data, &m) != nil {
		return Metadata{}, errors.New("invalid snapshot metadata")
	}
	if m.Format != 1 || m.Identity != s.Identity || m.Info.ClusterID != s.Identity.Cluster || m.Info.Version == "" || m.Info.Revision <= 0 || m.Bytes <= 0 || m.Created.IsZero() {
		return Metadata{}, errors.New("snapshot ownership or version evidence invalid")
	}
	f, err := os.Open(archive)
	if err != nil {
		return Metadata{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, &contextReader{ctx: ctx, Reader: f})
	if err != nil {
		return Metadata{}, err
	}
	if n != m.Bytes || hex.EncodeToString(h.Sum(nil)) != m.SHA256 {
		return Metadata{}, errors.New("snapshot checksum or size mismatch")
	}
	return m, nil
}

type contextReader struct {
	ctx context.Context
	io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.Reader.Read(p)
}

// Restorer must implement isolated etcdutl snapshot restore semantics: never
// execute inside the running distroless etcd container.
type Restorer interface {
	RestoreIsolated(context.Context, string, string, Identity, SnapshotInfo) error
	VerifyIsolated(context.Context, string, Identity, SnapshotInfo) error
}
type RestorePlan struct {
	Archive, Destination string
	Identity             Identity
	Info                 SnapshotInfo
	SHA256               string
}

func (s Store) PrepareRestore(ctx context.Context, destination string) (RestorePlan, error) {
	m, err := s.VerifySnapshot(ctx)
	if err != nil {
		return RestorePlan{}, err
	}
	if destination == "" || !filepath.IsAbs(destination) || filepath.Clean(destination) != destination {
		return RestorePlan{}, errors.New("absolute clean isolated destination required")
	}
	if err := rejectSymlinkAncestors(destination); err != nil {
		return RestorePlan{}, err
	}
	if _, err := os.Lstat(destination); err == nil {
		return RestorePlan{}, errors.New("restore destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return RestorePlan{}, err
	}
	archive, _, _ := s.paths()
	return RestorePlan{Archive: archive, Destination: destination, Identity: s.Identity, Info: m.Info, SHA256: m.SHA256}, nil
}
func (s Store) Restore(ctx context.Context, plan RestorePlan, client Restorer) error {
	if client == nil {
		return errors.New("isolated restore client required")
	}
	expected, err := s.PrepareRestore(ctx, plan.Destination)
	if err != nil {
		return err
	}
	if expected != plan {
		return errors.New("restore plan identity or checksum changed")
	}
	if err := client.RestoreIsolated(ctx, plan.Archive, plan.Destination, plan.Identity, plan.Info); err != nil {
		return fmt.Errorf("isolated restore failed; manual cleanup required: %w", err)
	}
	return nil
}
func (s Store) VerifyRecovery(ctx context.Context, plan RestorePlan, client Restorer) error {
	if client == nil {
		return errors.New("restore verifier required")
	}
	if plan.Identity != s.Identity {
		return errors.New("foreign recovery identity")
	}
	if plan.Destination == "" || !filepath.IsAbs(plan.Destination) || filepath.Clean(plan.Destination) != plan.Destination {
		return errors.New("invalid recovery destination")
	}
	if err := rejectSymlinkAncestors(plan.Destination); err != nil {
		return err
	}
	archive, _, err := s.paths()
	if err != nil {
		return err
	}
	if plan.Archive != archive {
		return errors.New("recovery plan refers to foreign snapshot")
	}
	m, err := s.VerifySnapshot(ctx)
	if err != nil {
		return err
	}
	if m.SHA256 != plan.SHA256 || m.Info != plan.Info {
		return errors.New("snapshot changed since restore")
	}
	return client.VerifyIsolated(ctx, plan.Destination, plan.Identity, plan.Info)
}

// rejectSymlinkAncestors rejects symlink redirection of an otherwise owner-only
// snapshot or restore path, including an existing ancestor of a new directory.
func rejectSymlinkAncestors(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("absolute path required")
	}
	clean := filepath.Clean(path)
	for current := clean; ; current = filepath.Dir(current) {
		st, err := os.Lstat(current)
		if err == nil && st.Mode()&os.ModeSymlink != 0 {
			return errors.New("symlink in managed recovery path")
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if parent := filepath.Dir(current); parent == current {
			break
		}
	}
	return nil
}

// RecoveryReadiness is supplied by the Core/Patroni orchestrator. Verification
// must prove both restored etcd quorum and the Patroni DCS consumer state.
type RecoveryReadiness interface {
	VerifyEtcdQuorum(context.Context, Identity, SnapshotInfo) error
	VerifyPatroniDCS(context.Context, Identity, SnapshotInfo) error
}

// VerifyOperationalRecovery never treats local files as proof of HA recovery.
func (s Store) VerifyOperationalRecovery(ctx context.Context, plan RestorePlan, client Restorer, readiness RecoveryReadiness) error {
	if readiness == nil {
		return errors.New("Core etcd quorum and Patroni readiness verifier required")
	}
	if err := s.VerifyRecovery(ctx, plan, client); err != nil {
		return err
	}
	if err := readiness.VerifyEtcdQuorum(ctx, plan.Identity, plan.Info); err != nil {
		return fmt.Errorf("restored etcd quorum not verified: %w", err)
	}
	if err := readiness.VerifyPatroniDCS(ctx, plan.Identity, plan.Info); err != nil {
		return fmt.Errorf("Patroni DCS readiness not verified: %w", err)
	}
	return nil
}
