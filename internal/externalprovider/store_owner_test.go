package externalprovider

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestStoreRejectsForeignAndMultiplyLinkedOwners(t *testing.T) {
	for _, mode := range []string{"registry-hardlink", "lock-hardlink", "registry-foreign-uid", "lock-foreign-uid"} {
		t.Run(mode, func(t *testing.T) {
			store := Store{Path: filepath.Join(t.TempDir(), "external-providers.json")}
			path := store.Path
			if mode == "lock-hardlink" || mode == "lock-foreign-uid" {
				path += ".lock"
			}
			original := []byte(`{"version":1}`)
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			foreign := filepath.Join(filepath.Dir(path), "foreign-owner")
			if mode == "registry-hardlink" || mode == "lock-hardlink" {
				if err := os.Link(path, foreign); err != nil {
					t.Fatal(err)
				}
			} else {
				if os.Geteuid() != 0 {
					t.Skip("foreign UID fixture requires local chown authority")
				}
				if err := os.Chown(path, 65534, -1); err != nil {
					if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.EPERM) {
						t.Skip("filesystem does not support the foreign UID fixture")
					}
					t.Fatal(err)
				}
			}
			if path == store.Path {
				if _, err := store.Load(); err == nil {
					t.Fatal("registry adopted a foreign or multiply linked owner")
				}
			}
			if err := store.Register(registryIntegrityFixture()); err == nil {
				t.Fatal("registry mutation accepted a foreign or multiply linked owner")
			}
			actual, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(actual, original) {
				t.Fatal("rejected owner changed before recovery")
			}
			if mode == "registry-hardlink" || mode == "lock-hardlink" {
				actual, err = os.ReadFile(foreign)
				if err != nil || !bytes.Equal(actual, original) {
					t.Fatal("rejected foreign hardlink changed")
				}
			}
		})
	}
}

type registryOwnerInfo struct {
	os.FileInfo
	stat any
}

func (info registryOwnerInfo) Sys() any { return info.stat }

func TestStoreOwnerRequiresVerifiedCurrentUIDAndSingleLink(t *testing.T) {
	for _, stat := range []any{
		nil,
		&syscall.Stat_t{Uid: uint32(os.Geteuid() + 1), Nlink: 1},
		&syscall.Stat_t{Uid: uint32(os.Geteuid()), Nlink: 2},
		&syscall.Stat_t{Uid: uint32(os.Geteuid()), Nlink: 0},
	} {
		if err := checkStoreOwner(registryOwnerInfo{stat: stat}); err == nil {
			t.Fatal("unknown, foreign or shared owner metadata accepted")
		}
	}
	if err := checkStoreOwner(registryOwnerInfo{stat: &syscall.Stat_t{Uid: uint32(os.Geteuid()), Nlink: 1}}); err != nil {
		t.Fatalf("verified exclusive owner rejected: %v", err)
	}
}
