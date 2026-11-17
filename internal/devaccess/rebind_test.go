package devaccess

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// stubManagedAuthority reports whether a managed object exists for a target and
// records whether it was consulted. It stands in for the OpenBao authority in
// the two recovery cells without touching a runtime.
type stubManagedAuthority struct {
	exists    bool
	consulted bool
}

func (s *stubManagedAuthority) Exists(_ context.Context, _ string) (bool, error) {
	s.consulted = true
	return s.exists, nil
}

// stubLocalStore records persist calls and writes the local opt-in credential to
// a temp dir so the same store can be re-read on rebind.
type stubLocalStore struct {
	path      string
	persisted bool
}

func (s *stubLocalStore) Persist(_ context.Context, target, username, password string) error {
	s.persisted = true
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path, []byte("USERNAME="+username+"\nPASSWORD="+password+"\n"), 0o600)
}

func TestRecoveryFailsClosedWhenNoLocalOptInCredential(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	// No managed authority and no local store wired: a fresh device with no
	// recoverable authority fails closed.
	recovery := Recovery{}

	result, err := recovery.Recover(context.Background(), "absenttarget")
	if !errors.Is(err, ErrNoRecoverableCredential) {
		t.Fatalf("recover with absent local store err = %v, want ErrNoRecoverableCredential (fail-closed)", err)
	}
	if result != Unrecovered {
		t.Fatalf("recovery result = %q, want %q (fail-closed without opt-in)", result, Unrecovered)
	}
}

func TestRecoveryUsesExistingManagedAuthorityWithoutDuplication(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	// Seed an opt-in local credential on the target via the real store.
	if _, err := Configure("isolated", devEnvironment, "developer", "local-opt-in"); err != nil {
		t.Fatal(err)
	}

	authority := &stubManagedAuthority{exists: true}
	store := &stubLocalStore{path: filepath.Join(root, "dev", "credentials.env")}
	recovery := Recovery{Managed: authority, Store: store}

	result, err := recovery.Recover(context.Background(), "isolated")
	if err != nil {
		t.Fatalf("recover returned error: %v", err)
	}
	if result != Recovered {
		t.Fatalf("recovery result = %q, want %q (managed object present)", result, Recovered)
	}
	if !authority.consulted {
		t.Fatal("managed authority was not consulted")
	}
	if store.persisted {
		t.Fatal("recovery duplicated an existing managed object instead of using it as-is")
	}
}

func TestRecoverySeedsManagedAuthorityOnceForFreshDevice(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	// Seed an opt-in local credential on the target via the real store.
	if _, err := Configure("isolated", devEnvironment, "developer", "local-opt-in"); err != nil {
		t.Fatal(err)
	}

	authority := &stubManagedAuthority{exists: false} // fresh device, no managed object yet
	store := &stubLocalStore{path: filepath.Join(root, "dev", "credentials.env")}
	recovery := Recovery{Managed: authority, Store: store}

	result, err := recovery.Recover(context.Background(), "isolated")
	if err != nil {
		t.Fatalf("recover returned error: %v", err)
	}
	if result != Recovered {
		t.Fatalf("recovery result = %q, want %q (fresh device seeds managed authority)", result, Recovered)
	}
	if !authority.consulted {
		t.Fatal("managed authority was not consulted")
	}
	if !store.persisted {
		t.Fatal("recovery did not seed the managed authority from the local opt-in credential")
	}
}

func TestRecoveryFailsClosedWithoutLocalStoreWhenNoManagedObject(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	authority := &stubManagedAuthority{exists: false}
	recovery := Recovery{Managed: authority} // no local store wired

	// Fresh target: no managed object, no local store -> fail closed.
	result, err := recovery.Recover(context.Background(), "freshdevice")
	if err == nil {
		t.Fatal("recovery accepted a fresh device with no recoverable authority")
	}
	if result != Unrecovered {
		t.Fatalf("recovery result = %q, want %q (fail-closed)", result, Unrecovered)
	}
}
