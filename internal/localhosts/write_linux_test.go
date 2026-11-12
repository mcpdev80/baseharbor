//go:build linux

package localhosts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHostsFileWriterPreservesInodeModeAndSymlinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	initial := []byte("127.0.0.1 localhost\n")
	if err := os.WriteFile(path, initial, 0640); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	id := ID("test", "target")
	block, _ := Block(id, "192.0.2.4", []string{"app.example.test"})
	if err := applyFile(path, Change{ID: id, Next: block}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if !os.SameFile(before, after) || before.Mode() != after.Mode() {
		t.Fatal("inode or mode changed")
	}
	if err := applyFile(path, Change{ID: id, Previous: block}); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	if string(body) != string(initial) {
		t.Fatal("original not restored")
	}
	link := path + "-link"
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if err := applyFile(link, Change{ID: id, Next: block}); err == nil {
		t.Fatal("symlink accepted")
	}
}
