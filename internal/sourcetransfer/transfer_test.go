package sourcetransfer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func putFixture(t *testing.T, root, path, value string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(value), 0644); err != nil {
		t.Fatal(err)
	}
}
func upload(t *testing.T, s *Store, m Manifest, contents map[string][]byte) (int, string) {
	t.Helper()
	id, paths, err := s.Begin(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		b := contents[p]
		for start := 0; start < len(b); {
			end := start + 11
			if end > len(b) {
				end = len(b)
			}
			if err := s.Put(id, p, int64(start), b[start:end]); err != nil {
				t.Fatal(err)
			}
			start = end
		}
		if len(b) == 0 {
			if err := s.Put(id, p, 0, nil); err != nil {
				t.Fatal(err)
			}
		}
	}
	ret, root, err := s.Commit(id)
	if err != nil {
		t.Fatal(err)
	}
	if ret.Digest != m.Digest {
		t.Fatal("commit digest differs")
	}
	return len(paths), root
}
func TestDirtySnapshotUploadIncrementalAndDelete(t *testing.T) {
	root := t.TempDir()
	putFixture(t, root, "baseharbor.yaml", "name: demo\n")
	putFixture(t, root, "cmd/start.sh", "echo first\n")
	putFixture(t, root, "untracked.txt", "new file")
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, data, err := Capture(context.Background(), root, "docker-dev", "demo", "dev", "")
	if err != nil {
		t.Fatal(err)
	}
	n, oldroot := upload(t, store, first, data)
	if n != 3 {
		t.Fatalf("initial upload = %d files", n)
	}
	putFixture(t, root, "cmd/start.sh", "echo second\n")
	if err := os.Remove(filepath.Join(root, "untracked.txt")); err != nil {
		t.Fatal(err)
	}
	second, data2, err := Capture(context.Background(), root, "docker-dev", "demo", "dev", first.Digest)
	if err != nil {
		t.Fatal(err)
	}
	n, newroot := upload(t, store, second, data2)
	if n != 1 {
		t.Fatalf("delta upload = %d files, wanted only changed", n)
	}
	if _, err := os.Stat(filepath.Join(newroot, "untracked.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("deleted file survived delta")
	}
	if b, err := os.ReadFile(filepath.Join(newroot, "baseharbor.yaml")); err != nil || string(b) != "name: demo\n" {
		t.Fatal("unchanged file was not retained")
	}
	if b, err := os.ReadFile(filepath.Join(oldroot, "cmd/start.sh")); err != nil || string(b) != "echo first\n" {
		t.Fatal("old snapshot was modified")
	}
}
func TestConflictsInvalidInputAndChunkIntegrity(t *testing.T) {
	root := t.TempDir()
	putFixture(t, root, "a.txt", "123456789")
	s, _ := NewStore(t.TempDir())
	m, data, err := Capture(context.Background(), root, "target", "demo", "dev", "")
	if err != nil {
		t.Fatal(err)
	}
	id, needed, err := s.Begin(m)
	if err != nil || !reflect.DeepEqual(needed, []string{"a.txt"}) {
		t.Fatalf("begin %v %v", needed, err)
	}
	if err := s.Put(id, "a.txt", 5, []byte("wrong")); err == nil {
		t.Fatal("out-of-order accepted")
	}
	if err := s.Put(id, "../escape", 0, []byte("bad")); err == nil {
		t.Fatal("traversal accepted")
	}
	if err := s.Put(id, "a.txt", 0, []byte("123456780")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Commit(id); err == nil {
		t.Fatal("corrupt bytes committed")
	}
	if err := s.Abort(id); err != nil {
		t.Fatal(err)
	}
	upload(t, s, m, data)
	old := m
	old.Base = ""
	if _, _, err := s.Begin(old); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale base accepted: %v", err)
	}
	for _, p := range []string{"../secret", "a/../../secret", ".git/config", ".env", ".env.production", ".npmrc", ".netrc", ".baseharbor/tokens.json", "a\\b"} {
		if err := validatePath(p); err == nil {
			t.Fatalf("accepted %q", p)
		}
	}
}
func TestSymlinkAndConfidentialPathRejected(t *testing.T) {
	root := t.TempDir()
	putFixture(t, root, "a.txt", "a")
	if err := os.Symlink("a.txt", filepath.Join(root, "shadow")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Capture(context.Background(), root, "dev", "demo", "dev", ""); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := os.Remove(filepath.Join(root, "shadow")); err != nil {
		t.Fatal(err)
	}
	putFixture(t, root, ".env", "PASSWORD=secret")
	if _, _, err := Capture(context.Background(), root, "dev", "demo", "dev", ""); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("secret path not denied: %v", err)
	}
}

func TestSourceCaptureDoesNotTransmitLocalInfrastructureSecrets(t *testing.T) {
	root := t.TempDir()
	putFixture(t, root, "baseharbor.yaml", "name: demo\n")
	for _, name := range []string{
		".baseharbor/operator-token.json",
		".aws/credentials", ".ssh/id_ed25519", ".kube/config",
		".docker/config.json", ".terraform/terraform.tfstate",
	} {
		putFixture(t, root, name, "DO_NOT_UPLOAD")
	}
	snapshot, contents, err := Capture(context.Background(), root, "dev", "demo", "dev", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Files) != 1 || len(contents) != 1 || snapshot.Files[0].Path != "baseharbor.yaml" {
		t.Fatalf("local infrastructure secret was captured: %v", snapshot.Files)
	}
}
