package sourcetransfer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const Version = "baseharbor.source-transfer/v1"
const MaxFileBytes int64 = 32 << 20
const MaxTotalBytes int64 = 256 << 20
const MaxFiles = 4096
const MaxChunkBytes = 512 << 10

var ErrConflict = errors.New("source transfer is stale or conflicts with the active snapshot")

// Entry binds a source path, contents, mode and size. No symlinks or special files.
type Entry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Mode   uint32 `json:"mode"`
}

type Manifest struct {
	Version     string  `json:"version"`
	Target      string  `json:"target"`
	Application string  `json:"application"`
	Environment string  `json:"environment"`
	Base        string  `json:"base,omitempty"`
	Files       []Entry `json:"files"`
	Digest      string  `json:"digest"`
}

// The validated snapshot is a separate immutable content directory, not a live checkout.
// The previous digest is an optimistic concurrency guard, protecting concurrent edits.
func (m Manifest) Validate() error {
	if m.Version != Version || !validSelector(m.Target) || !validSelector(m.Application) || !validSelector(m.Environment) || len(m.Files) == 0 || len(m.Files) > MaxFiles {
		return errors.New("invalid source transfer identity or file count")
	}
	if m.Base != "" && !validDigest(m.Base) {
		return errors.New("invalid base digest")
	}
	seen := map[string]bool{}
	var total int64
	last := ""
	for _, f := range m.Files {
		if err := validatePath(f.Path); err != nil {
			return err
		}
		if seen[f.Path] || (last != "" && last >= f.Path) || !validDigest(f.SHA256) || f.Size < 0 || f.Size > MaxFileBytes || (f.Mode != 0600 && f.Mode != 0644 && f.Mode != 0700 && f.Mode != 0755) {
			return fmt.Errorf("invalid source transfer entry %q", f.Path)
		}
		seen[f.Path] = true
		last = f.Path
		total += f.Size
		if total > MaxTotalBytes {
			return errors.New("source snapshot exceeds byte limit")
		}
	}
	digest, err := manifestDigest(m)
	if err != nil {
		return err
	}
	if m.Digest != digest {
		return errors.New("source manifest digest does not match canonical content")
	}
	return nil
}
func validDigest(v string) bool {
	if len(v) != 64 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil && strings.ToLower(v) == v
}
func validSelector(v string) bool {
	if len(v) == 0 || len(v) > 128 || v == "." || v == ".." {
		return false
	}
	for _, r := range v {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}
func validatePath(p string) error {
	if p == "" || p == "." || path.Clean(p) != p || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "../") || strings.ContainsAny(p, "\\\x00\r\n") {
		return fmt.Errorf("invalid source path %q", p)
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." || part == ".git" || part == ".env" || part == ".ssh" || part == ".aws" || part == ".gnupg" || p == ".baseharbor/init.env" {
			return fmt.Errorf("source path %q contains forbidden material", p)
		}
	}
	return nil
}
func manifestDigest(m Manifest) (string, error) {
	m.Digest = ""
	m.Base = "" // optimistic-concurrency base is not part of immutable source identity
	raw, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(raw)
	return hex.EncodeToString(s[:]), nil
}
func digestBytes(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func CreateManifest(target, application, environment, base string, entries []Entry) (Manifest, error) {
	sorted := append([]Entry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	m := Manifest{Version: Version, Target: target, Application: application, Environment: environment, Base: base, Files: sorted}
	m.Digest, _ = manifestDigest(m)
	return m, m.Validate()
}

// Capture includes uncommitted and untracked regular files. Excluded cache,
// Git metadata and secret directories are never transmitted. Any symlink fails
// closed, avoiding traversal into an unowned filesystem.
func Capture(ctx context.Context, root, target, app, environment, base string) (Manifest, map[string][]byte, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Manifest{}, nil, err
	}
	var entries []Entry
	contents := map[string][]byte{}
	var total int64
	err = filepath.WalkDir(root, func(filename string, d os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		p := filepath.ToSlash(rel)
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", ".next", "dist", "build", ".venv", "venv", "__pycache__":
				return filepath.SkipDir
			}
			return validatePath(p)
		}
		if err := validatePath(p); err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("source contains symlink or special file %q", p)
		}
		f, err := os.Open(filename)
		if err != nil {
			return err
		}
		info, err := f.Stat()
		if err != nil {
			f.Close()
			return err
		}
		if info.Size() > MaxFileBytes || len(entries) >= MaxFiles {
			f.Close()
			return errors.New("source file count or size exceeds limit")
		}
		b, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if int64(len(b)) != info.Size() {
			return fmt.Errorf("source changed while capturing %q", p)
		}
		again, err := os.Lstat(filename)
		if err != nil {
			return err
		}
		if !again.Mode().IsRegular() || !again.ModTime().Equal(info.ModTime()) || again.Size() != info.Size() {
			return fmt.Errorf("source changed during capture %q", p)
		}
		total += int64(len(b))
		if total > MaxTotalBytes {
			return errors.New("source snapshot exceeds byte limit")
		}
		mode := uint32(0644)
		if info.Mode().Perm()&0111 != 0 {
			mode = 0755
		}
		if info.Mode().Perm()&0077 == 0 {
			mode = 0600
		}
		entries = append(entries, Entry{Path: p, SHA256: digestBytes(b), Size: int64(len(b)), Mode: mode})
		contents[p] = b
		return nil
	})
	if err != nil {
		return Manifest{}, nil, err
	}
	m, err := CreateManifest(target, app, environment, base, entries)
	return m, contents, err
}

type activeTransfer struct {
	manifest Manifest
	root     string
	next     map[string]int64
	needed   map[string]bool
}
type Store struct {
	mu     sync.Mutex
	root   string
	active map[string]*activeTransfer
}

func NewStore(root string) (*Store, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absolute, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(absolute, 0700); err != nil {
		return nil, err
	}
	return &Store{root: absolute, active: map[string]*activeTransfer{}}, nil
}
func (s *Store) namespace(m Manifest) string {
	return filepath.Join(s.root, m.Target, m.Application, m.Environment)
}
func (s *Store) Current(target, app, env string) (Manifest, string, error) {
	if !validSelector(target) || !validSelector(app) || !validSelector(env) {
		return Manifest{}, "", errors.New("invalid source identity")
	}
	ns := filepath.Join(s.root, target, app, env)
	raw, err := os.ReadFile(filepath.Join(ns, "current.json"))
	if err != nil {
		return Manifest{}, "", err
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Manifest{}, "", err
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, "", err
	}
	if m.Target != target || m.Application != app || m.Environment != env {
		return Manifest{}, "", ErrConflict
	}
	return m, filepath.Join(ns, "objects", m.Digest), nil
}

// Begin compares file hashes against the previously committed base. Returned
// paths are the only bytes the developer needs to transmit for a delta.
func (s *Store) Begin(m Manifest) (string, []string, error) {
	if err := m.Validate(); err != nil {
		return "", nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old, oldRoot, err := s.Current(m.Target, m.Application, m.Environment)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", nil, err
	}
	if errors.Is(err, os.ErrNotExist) {
		if m.Base != "" {
			return "", nil, ErrConflict
		}
	} else if m.Base != old.Digest {
		return "", nil, ErrConflict
	}
	ns := s.namespace(m)
	if err := os.MkdirAll(filepath.Join(ns, "pending"), 0700); err != nil {
		return "", nil, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", nil, err
	}
	id := hex.EncodeToString(random[:])
	tmp := filepath.Join(ns, "pending", id)
	if err := os.Mkdir(tmp, 0700); err != nil {
		return "", nil, err
	}
	need := map[string]bool{}
	next := map[string]int64{}
	var paths []string
	matched := map[string]Entry{}
	if old.Digest != "" {
		for _, f := range old.Files {
			matched[f.Path] = f
		}
	}
	for _, f := range m.Files {
		if previous, ok := matched[f.Path]; ok && previous.SHA256 == f.SHA256 && previous.Size == f.Size {
			src := filepath.Join(oldRoot, filepath.FromSlash(f.Path))
			data, readErr := os.ReadFile(src)
			if readErr == nil && int64(len(data)) == f.Size && digestBytes(data) == f.SHA256 {
				if err := writeOwnedFile(tmp, f.Path, data, f.Mode); err != nil {
					os.RemoveAll(tmp)
					return "", nil, err
				}
				continue
			}
		}
		need[f.Path] = true
		next[f.Path] = 0
		paths = append(paths, f.Path)
	}
	s.active[id] = &activeTransfer{manifest: m, root: tmp, next: next, needed: need}
	return id, paths, nil
}
func writeOwnedFile(root, p string, data []byte, mode uint32) error {
	dest := filepath.Join(root, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	// Paths from a validated canonical manifest never traverse out of root.
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(mode))
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func (s *Store) Put(id, p string, offset int64, chunk []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, ok := s.active[id]
	if !ok {
		return errors.New("unknown source transfer")
	}
	if !tx.needed[p] || offset != tx.next[p] || len(chunk) > MaxChunkBytes {
		return errors.New("unexpected source transfer chunk")
	}
	var f Entry
	for _, entry := range tx.manifest.Files {
		if entry.Path == p {
			f = entry
			break
		}
	}
	if offset+int64(len(chunk)) > f.Size || (len(chunk) == 0 && f.Size != 0) {
		return errors.New("source chunk exceeds declared size")
	}
	dest := filepath.Join(tx.root, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY, os.FileMode(f.Mode))
	if err != nil {
		return err
	}
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		file.Close()
		return err
	}
	n, err := file.Write(chunk)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if n != len(chunk) {
		return io.ErrShortWrite
	}
	tx.next[p] += int64(n)
	return nil
}
func (s *Store) Commit(id string) (Manifest, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, ok := s.active[id]
	if !ok {
		return Manifest{}, "", errors.New("unknown source transfer")
	}
	for _, f := range tx.manifest.Files {
		if tx.needed[f.Path] && tx.next[f.Path] != f.Size {
			return Manifest{}, "", errors.New("source transfer is incomplete")
		}
		content, err := os.ReadFile(filepath.Join(tx.root, filepath.FromSlash(f.Path)))
		if err != nil || int64(len(content)) != f.Size || digestBytes(content) != f.SHA256 {
			return Manifest{}, "", fmt.Errorf("source digest differs for %q", f.Path)
		}
	}
	previous, _, err := s.Current(tx.manifest.Target, tx.manifest.Application, tx.manifest.Environment)
	if errors.Is(err, os.ErrNotExist) {
		if tx.manifest.Base != "" {
			return Manifest{}, "", ErrConflict
		}
	} else if err != nil {
		return Manifest{}, "", err
	} else if previous.Digest != tx.manifest.Base {
		return Manifest{}, "", ErrConflict
	}
	ns := s.namespace(tx.manifest)
	objects := filepath.Join(ns, "objects")
	if err := os.MkdirAll(objects, 0700); err != nil {
		return Manifest{}, "", err
	}
	destination := filepath.Join(objects, tx.manifest.Digest)
	if err := os.Rename(tx.root, destination); err != nil {
		return Manifest{}, "", err
	}
	raw, err := json.Marshal(tx.manifest)
	if err != nil {
		return Manifest{}, "", err
	}
	pointer := filepath.Join(ns, "current.json")
	tmp := pointer + ".tmp" + id
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return Manifest{}, "", err
	}
	if err := os.Rename(tmp, pointer); err != nil {
		return Manifest{}, "", err
	}
	delete(s.active, id)
	return tx.manifest, destination, nil
}

// Pending identifies the exact authorized target/application scope for a live
// transfer. The HTTP gateway must reauthorize every chunk and commit.
func (s *Store) Pending(id string) (Manifest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, ok := s.active[id]
	if !ok {
		return Manifest{}, errors.New("unknown source transfer")
	}
	return tx.manifest, nil
}

func (s *Store) Abort(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, ok := s.active[id]
	if !ok {
		return errors.New("unknown source transfer")
	}
	delete(s.active, id)
	return os.RemoveAll(tx.root)
}
