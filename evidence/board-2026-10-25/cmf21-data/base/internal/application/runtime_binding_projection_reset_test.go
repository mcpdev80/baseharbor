package application

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBindingProjectionResetKeepsMountedRootAndOnlySelectedIdentity(t *testing.T) {
	for _, selected := range []bool{true, false} {
		t.Run(map[bool]string{true: "selected", false: "removed"}[selected], func(t *testing.T) {
			root := protectedTempDir(t)
			before, err := os.Stat(root)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{IdentityBindingName, "postgresql", "unknown"} {
				if err := os.Mkdir(filepath.Join(root, name), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, name, "material"), []byte("retained"), 0444); err != nil {
					t.Fatal(err)
				}
			}
			m := Manifest{Services: Services{Identity: selected}}
			for range 2 {
				if err := resetWorkloadBindingProjection(m, root); err != nil {
					t.Fatal(err)
				}
			}
			after, err := os.Stat(root)
			if err != nil || !os.SameFile(before, after) {
				t.Fatal("mounted root replaced")
			}
			for _, name := range []string{IdentityBindingName, "postgresql", "unknown"} {
				data, err := os.ReadFile(filepath.Join(root, name, "material"))
				if name == IdentityBindingName && selected {
					if err != nil || string(data) != "retained" {
						t.Fatal("selected independent material lost")
					}
				} else if !os.IsNotExist(err) {
					t.Fatal("stale projection retained")
				}
			}
		})
	}
}

func TestBindingProjectionResetRejectsRedirectedOrWritableOwnersBeforeMutation(t *testing.T) {
	for _, bad := range []string{"root-link", "identity-link", "identity-file", "root-writable", "identity-writable"} {
		t.Run(bad, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "projection")
			outside := filepath.Join(base, "outside")
			for _, dir := range []string{root, outside} {
				if err := os.Mkdir(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			marker := filepath.Join(outside, "marker")
			if err := os.WriteFile(marker, []byte("outside-owner"), 0600); err != nil {
				t.Fatal(err)
			}
			identity := filepath.Join(root, IdentityBindingName)
			switch bad {
			case "root-link":
				if err := os.Remove(root); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, root); err != nil {
					t.Fatal(err)
				}
			case "identity-link":
				if err := os.Symlink(outside, identity); err != nil {
					t.Fatal(err)
				}
			case "identity-file":
				if err := os.WriteFile(identity, nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "root-writable":
				if err := os.Chmod(root, 0777); err != nil {
					t.Fatal(err)
				}
			case "identity-writable":
				if err := os.Mkdir(identity, 0777); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(identity, 0777); err != nil {
					t.Fatal(err)
				}
			}
			backend := filepath.Join(root, "postgresql")
			if err := os.WriteFile(backend, []byte("existing-backend"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := resetWorkloadBindingProjection(Manifest{Services: Services{Identity: true}}, root); err == nil {
				t.Fatal("unverifiable owner accepted")
			}
			for path, expected := range map[string]string{marker: "outside-owner", backend: "existing-backend"} {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != expected {
					t.Fatal("rejected projection changed existing owner")
				}
			}
		})
	}
}

func TestBindingProjectionResetRemovesBackendAliasWithoutFollowingIt(t *testing.T) {
	root := protectedTempDir(t)
	outside := t.TempDir()
	marker := filepath.Join(outside, "marker")
	if err := os.WriteFile(marker, []byte("outside-owner"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "postgresql")); err != nil {
		t.Fatal(err)
	}
	if err := resetWorkloadBindingProjection(Manifest{}, root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "outside-owner" {
		t.Fatal("backend alias target changed")
	}
}
