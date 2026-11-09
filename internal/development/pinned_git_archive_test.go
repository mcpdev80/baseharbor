package development

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStagePinnedGitTreeExportsCommitNotDirtyWorktree(t *testing.T) {
	checkout := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", checkout}, args...)...)
		result, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, result)
		}
		return strings.TrimSpace(string(result))
	}
	git("init")
	file := filepath.Join(checkout, "compose.yaml")
	if err := os.WriteFile(file, []byte("services: original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "compose.yaml")
	git("-c", "user.name=Test", "-c", "user.email=test@example.org", "commit", "-m", "pinned")
	revision := git("rev-parse", "HEAD")
	if err := os.WriteFile(file, []byte("services: dirty\n"), 0600); err != nil {
		t.Fatal(err)
	}
	staged, err := StagePinnedGitTree(context.Background(), checkout, revision, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(staged)
	content, err := os.ReadFile(filepath.Join(staged, "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "services: original\n" {
		t.Fatalf("staged tree took dirty checkout instead of immutable commit: %q", content)
	}
	if _, err := StagePinnedGitTree(context.Background(), checkout, "main", t.TempDir()); err == nil {
		t.Fatal("mutable Git branch was accepted as archive revision")
	}
}

func TestExtractPinnedGitArchiveRefusesUnsafeEntries(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		kind byte
	}{
		{"parent-path", "../escape", tar.TypeReg},
		{"absolute-path", "/tmp/escape", tar.TypeReg},
		{"symlink", "evil", tar.TypeSymlink},
		{"git-metadata", ".git/config", tar.TypeReg},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buffer bytes.Buffer
			writer := tar.NewWriter(&buffer)
			if err := writer.WriteHeader(&tar.Header{Name: tc.path, Mode: 0600, Typeflag: tc.kind, Linkname: "../escape"}); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := extractPinnedGitArchive(&buffer, t.TempDir()); err == nil {
				t.Fatal("unsafe Git archive entry was accepted")
			}
		})
	}
}
