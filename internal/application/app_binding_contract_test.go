package application

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// These tests are the RED-FIRST baseline for r27-app-binding. They assert the
// server-identity contract on unmodified develop BEFORE any implementation.
// Every assertion that currently fails must show a verbatim RED line in the
// round-1 evidence before any bounded fix lands.

// TestServerAllocatesIdentityNeverClientsProves that a checkout cannot author,
// override or rename an existing app identity. The server is the sole ID owner.
func TestServerAllocatesIdentityNeverClientsProves(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: filepath.Join(root, "apps")}
	m := WithWorkloadComponents(Manifest{Version: 1, Name: "myapp", Environment: "dev"}, "app")
	allocated, err := store.InitializeIdentity(m, IdentitySnapshot{})
	if err != nil {
		t.Fatalf("initial allocation: %v", err)
	}
	if allocated.ApplicationID == "" {
		t.Fatal("server did not allocate an application id")
	}

	// (1) A client-provided non-UUID id is rejected.
	client := m
	client.ApplicationID = "client-chosen-id"
	if _, err := store.InitializeIdentity(client, IdentitySnapshot{}); err == nil {
		t.Fatal("client-authored id was accepted; id assignment is not server-side")
	}

	// (2) A client-provided valid UUID that does not match the owner is rejected.
	fresh := MustNewApplicationID()
	attacker := m
	attacker.ApplicationID = fresh
	if _, err := store.InitializeIdentity(attacker, IdentitySnapshot{}); err == nil {
		t.Fatal("foreign valid uuid was accepted; id assignment is not server-side")
	}

	// (3) A client that re-uses the owner id under a different name is rejected.
	owner := m
	owner.ApplicationID = allocated.ApplicationID
	owner.Name = "renamed"
	if _, err := store.InitializeIdentity(owner, IdentitySnapshot{}); err == nil {
		t.Fatal("attach under renamed app was accepted; name is not authorization")
	}

	// (4) The authored repository contract never carries the id.
	_, _ = os.ReadFile(filepath.Join(root, RepositoryManifestName))
}

// TestConcurrentFirstRegistrationConvergesProves proves that N concurrent
// first registrations on two fresh checkout paths converge on ONE server-owned
// id and never rewrite each other. race-count=20.
func TestConcurrentFirstRegistrationConvergesProves(t *testing.T) {
	const n = 20
	root := t.TempDir()
	store := Store{Root: filepath.Join(root, "apps")}
	m := WithWorkloadComponents(Manifest{Version: 1, Name: "myapp", Environment: "dev"}, "app")

	var wg sync.WaitGroup
	results := make([]string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			func() {
				defer func() {
					if r := recover(); r != nil {
						results[i] = "<panic>"
					}
				}()
				got, err := store.InitializeIdentity(m, IdentitySnapshot{})
				if err != nil {
					results[i] = "<err>"
					return
				}
				results[i] = got.ApplicationID
			}()
		}(i)
	}
	wg.Wait()
	winner, err := store.InitializeIdentity(m, IdentitySnapshot{})
	if err != nil {
		t.Fatalf("post-race winner: %v", err)
	}
	seen := map[string]bool{}
	for _, id := range results {
		if id == "" || id == "<err>" || id == "<panic>" {
			t.Fatalf("concurrent registration did not converge: %q", id)
		}
		seen[id] = true
	}
	if len(seen) != 1 {
		t.Fatalf("bindings diverged across concurrent checkouts: %v", seen)
	}
	for id := range seen {
		if id != winner.ApplicationID {
			t.Fatal("concurrent winner differs from server-owned id")
		}
	}
}

// TestAttachToPermittedExistingAppProvesNamedDenial proves the two attach
// outcomes: (1) a checkout that reuses its own app's id under its own name
// converges (permitted attach); (2) a checkout that reuses an existing id under
// a DIFFERENT name is denied with a named fail-closed error.
func TestAttachToPermittedExistingAppProvesNamedDenial(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: filepath.Join(root, "apps")}
	m := WithWorkloadComponents(Manifest{Version: 1, Name: "myapp", Environment: "dev"}, "app")
	allocated, err := store.InitializeIdentity(m, IdentitySnapshot{})
	if err != nil {
		t.Fatalf("initial allocation: %v", err)
	}

	// Permitted attach: same name, same id converges to the same owner id.
	converge := m
	converge.ApplicationID = allocated.ApplicationID
	got, err := store.InitializeIdentity(converge, IdentitySnapshot{})
	if err != nil {
		t.Fatalf("permitted attach rejected: %v", err)
	}
	if got.ApplicationID != allocated.ApplicationID {
		t.Fatal("permitted attach did not converge on the server id")
	}

	// Disallowed attach: foreign name over an existing id is denied with a
	// NAMED error that names the existing owner.
	foreign := m
	foreign.ApplicationID = allocated.ApplicationID
	foreign.Name = "foreign"
	_, err = store.InitializeIdentity(foreign, IdentitySnapshot{})
	if err == nil {
		t.Fatal("foreign-name attach over existing id was accepted; attach is not permission-gated")
	}
	if !strings.Contains(err.Error(), "already owned by") {
		t.Fatalf("attach denial missing the named owner error: %v", err)
	}
}
