package onboardingbundle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistIdentityDoesNotExposeOrReplacePrivateMaterial(t *testing.T) {
	_, invite := testCoreServer(t, false)
	now := time.Now()
	identity, err := Redeem(context.Background(), invite, now)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "private-workstations")
	path, err := PersistIdentity(root, invite, identity, now)
	if err != nil {
		t.Fatalf("persist: %v", err)
	}
	dir, err := os.Lstat(path)
	if err != nil || !dir.IsDir() || dir.Mode().Perm() != 0o700 {
		t.Fatalf("identity directory must be private: %v %v", dir, err)
	}
	for _, name := range []string{"private-key.pem", "certificate.pem", "ca.pem", "identity.json"} {
		stat, err := os.Lstat(filepath.Join(path, name))
		if err != nil || !stat.Mode().IsRegular() || stat.Mode().Perm() != 0o600 {
			t.Fatalf("%s was not owner-only: %v %v", name, stat, err)
		}
	}
	if _, err := PersistIdentity(root, invite, identity, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("overwrote an enrolled device: %v", err)
	}
}

func TestPersistIdentityRefusesForeignOrIncompleteMaterial(t *testing.T) {
	_, invite := testCoreServer(t, false)
	now := time.Now()
	bogus := EnrolledIdentity{
		InstallationID: invite.InstallationID,
		Station:        "other-station",
		Serial:         "22",
		ExpiresAt:      now.Add(time.Hour),
		PrivateKeyPEM:  []byte("not a key"),
		CertificatePEM: []byte("not a certificate"),
		TrustPEM:       []byte(invite.CAPEM),
	}
	root := filepath.Join(t.TempDir(), "device")
	if _, err := PersistIdentity(root, invite, bogus, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("foreign identity stored: %v", err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("rejected identity created a directory")
	}
}
