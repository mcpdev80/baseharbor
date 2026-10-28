package application

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateRuntimeWriteDoesNotAdoptPredictableTemporaryFiles(t *testing.T) {
	for _, linked := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing-temporary", true: "foreign-link"}[linked], func(t *testing.T) {
			root := t.TempDir()
			path, foreign := filepath.Join(root, "runtime.env"), filepath.Join(root, "foreign")
			if err := os.WriteFile(foreign, []byte("foreign recovery material"), 0600); err != nil {
				t.Fatal(err)
			}
			if linked {
				if err := os.Symlink(foreign, path+".tmp"); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path+".tmp", []byte("existing recovery material"), 0600); err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(path + ".tmp")
			if err != nil {
				t.Fatal(err)
			}
			if err := writeOwnerOnlyFile(path, []byte("selected owned material")); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(foreign)
			if err != nil || string(data) != "foreign recovery material" {
				t.Fatal("private writer modified foreign material")
			}
			after, err := os.Lstat(path + ".tmp")
			if err != nil || !os.SameFile(before, after) {
				t.Fatal("private writer adopted existing recovery material")
			}
			owner, err := os.Lstat(path)
			if err != nil || !owner.Mode().IsRegular() || owner.Mode().Perm() != 0600 {
				t.Fatal("private writer did not establish its protected regular owner")
			}
			data, err = os.ReadFile(path)
			if err != nil || string(data) != "selected owned material" {
				t.Fatal("private writer did not persist selected material")
			}
		})
	}
}

func TestPrivateRuntimeWriteRejectsRedirectedFinalOwner(t *testing.T) {
	root := t.TempDir()
	path, foreign := filepath.Join(root, "runtime.env"), filepath.Join(root, "foreign")
	if err := os.WriteFile(foreign, []byte("foreign recovery material"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, path); err != nil {
		t.Fatal(err)
	}
	if err := writeOwnerOnlyFile(path, []byte("selected owned material")); err == nil {
		t.Fatal("private writer adopted a redirected owner")
	}
	if data, err := os.ReadFile(foreign); err != nil || string(data) != "foreign recovery material" {
		t.Fatal("private writer changed rejected foreign material")
	}
	if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("private writer replaced rejected recovery owner")
	}
}

func TestPrivateRuntimeWriteRejectsRedirectedParentAndMultiplyLinkedOwner(t *testing.T) {
	for _, mode := range []string{"parent", "hardlink"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			foreign := filepath.Join(root, "foreign")
			if err := os.Mkdir(foreign, 0700); err != nil {
				t.Fatal(err)
			}
			original := filepath.Join(foreign, "runtime.env")
			if err := os.WriteFile(original, []byte("foreign recovery material"), 0600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "runtime.env")
			if mode == "parent" {
				parent := filepath.Join(root, "redirect")
				if err := os.Symlink(foreign, parent); err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(parent, "runtime.env")
			} else if err := os.Link(original, path); err != nil {
				t.Fatal(err)
			}
			if err := writeOwnerOnlyFile(path, []byte("selected owned material")); err == nil {
				t.Fatal("private writer adopted ambiguous owner")
			}
			if data, err := os.ReadFile(original); err != nil || string(data) != "foreign recovery material" {
				t.Fatal("private writer modified ambiguous material")
			}
		})
	}
}
