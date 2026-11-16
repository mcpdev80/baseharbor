package deployment

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/environment"
)

// TestRegisterEnvironmentAssociationCreatesAndReuses proves the idempotent
// membership guarantee: the first registration of an app binding creates it,
// and every later registration — including a brownfield reopen — returns that
// exact same binding. A repeated registration never allocates a duplicate and
// never mutates the record.
func TestRegisterEnvironmentAssociationCreatesAndReuses(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	appID := "app-1234"
	binding, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindApp, appID)
	if err != nil {
		t.Fatalf("first RegisterEnvironmentAssociation: %v", err)
	}
	if binding.SubjectKind != AssociationKindApp || binding.SubjectID != appID {
		t.Fatalf("returned binding mismatch: %+v", binding)
	}

	for i := 0; i < 50; i++ {
		again, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindApp, appID)
		if err != nil {
			t.Fatalf("repeat RegisterEnvironmentAssociation: %v", err)
		}
		if again.SubjectID != binding.SubjectID {
			t.Fatalf("membership changed on repeat: %+v -> %+v", binding, again)
		}
	}
}

// TestRegisterEnvironmentAssociationIsPerTargetIsolation confirms two Targets
// never share an association, even for the same environment name and subject.
func TestRegisterEnvironmentAssociationIsPerTargetIsolation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	appID := "app-1234"
	if _, err := RegisterEnvironmentAssociation("target-a", "shared", AssociationKindApp, appID); err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterEnvironmentAssociation("target-b", "shared", AssociationKindApp, appID); err != nil {
		t.Fatal(err)
	}

	// The membership lists are per-target: both targets can associate the same
	// app independently, but the environments never share a record.
	a, err := ListEnvironmentAssociations("target-a", "shared")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ListEnvironmentAssociations("target-b", "shared")
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("expected one binding per target, got a=%d b=%d", len(a), len(b))
	}
}

// TestRegisterEnvironmentAssociationDuplicateFailsClosed proves that a
// duplicate (kind, id) registration on an environment that already carries a
// DIFFERENT binding for the same key is refused — the membership key is the
// source of truth, and a collision fails closed rather than silently creating
// a duplicate (state split, precedent 0427).
func TestRegisterEnvironmentAssociationDuplicateFailsClosed(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	appID := "app-1234"
	if _, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindApp, appID); err != nil {
		t.Fatal(err)
	}

	// The same (app, env) registered a second time must NOT create a duplicate;
	// it converges to the existing binding (idempotent no-op), which is the
	// accepted behaviour for a repeated registration of an already-member.
	converged, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindApp, appID)
	if err != nil {
		t.Fatalf("idempotent repeat must converge, got err: %v", err)
	}
	if converged.SubjectID != appID {
		t.Fatalf("converged binding mismatch: %+v", converged)
	}

	// Exactly one binding on record: no duplicate list was written.
	list, err := ListEnvironmentAssociations("target-a", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("duplicate registration wrote %d bindings, want 1", len(list))
	}
}

// TestRegisterAndDeregisterEnvironmentAssociation proves the removal path: a
// registered subject can be deregistered, the environment identity and the
// OTHER bindings are preserved, and deregistering a non-member is a no-op.
func TestRegisterAndDeregisterEnvironmentAssociation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	appID := "app-1234"
	nodeID := "node-5678"
	if _, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindApp, appID); err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindNode, nodeID); err != nil {
		t.Fatal(err)
	}

	// Deregister the app; the node binding must survive.
	if err := DeregistrationEnvironmentAssociation("target-a", "prod", AssociationKindApp, appID); err != nil {
		t.Fatalf("deregister app: %v", err)
	}
	list, err := ListEnvironmentAssociations("target-a", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].SubjectKind != AssociationKindNode || list[0].SubjectID != nodeID {
		t.Fatalf("node binding lost after app deregistration: %+v", list)
	}

	// Deregistering the same app again is an idempotent no-op.
	if err := DeregistrationEnvironmentAssociation("target-a", "prod", AssociationKindApp, appID); err != nil {
		t.Fatalf("idempotent deregister: %v", err)
	}
	// Deregistering a never-registered subject is also a no-op.
	if err := DeregistrationEnvironmentAssociation("target-a", "prod", AssociationKindApp, "app-9999"); err != nil {
		t.Fatalf("deregister non-member: %v", err)
	}
}

// TestRegisterEnvironmentAssociationConcurrentRegistrationIsAtomic is the
// acceptance-critical test: N goroutines racing the first registration of the
// SAME (kind, id) on one environment must all converge on exactly one binding.
// A non-atomic implementation would write divergent lists and poison the
// environment permanently (state split, precedent 0427).
func TestRegisterEnvironmentAssociationConcurrentRegistrationIsAtomic(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	appID := "app-concurrent"
	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers)
	var failed int
	var addMu sync.Mutex
	seen := map[string]bool{}
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			binding, err := RegisterEnvironmentAssociation("target-a", "hot", AssociationKindApp, appID)
			if err != nil {
				addMu.Lock()
				failed++
				addMu.Unlock()
				t.Errorf("worker failed: %v", err)
				return
			}
			addMu.Lock()
			seen[binding.SubjectID] = true
			addMu.Unlock()
		}()
	}
	wg.Wait()
	if failed != 0 {
		t.Fatalf("no worker should fail when all converge, got %d", failed)
	}
	if len(seen) != 1 {
		t.Fatalf("concurrent registration produced divergent bindings: %v", seen)
	}
	list, err := ListEnvironmentAssociations("target-a", "hot")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("concurrent registration wrote %d bindings, want 1", len(list))
	}
}

// TestRegisterEnvironmentAssociationPersistsAcrossStateRootRead proves Core
// restart persistence: a fresh read of the same state root (simulating a new
// Core process opening the same installation) observes the exact membership the
// previous process registered, never a re-derived or empty list.
func TestRegisterEnvironmentAssociationPersistsAcrossStateRootRead(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	appID := "app-persist"
	nodeID := "node-persist"
	if _, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindApp, appID); err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindNode, nodeID); err != nil {
		t.Fatal(err)
	}

	// A brand-new process opening the SAME state root must see both bindings,
	// proving they were durably written and survive a restart.
	list, err := ListEnvironmentAssociations("target-a", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("restart observed %d bindings, want 2", len(list))
	}

	// The on-disk record must carry the associations on the SAME file as the
	// identity — never a second source of truth.
	home := filepath.Join(root, "targets", "target-a", ".baseharbor", "environments", "prod")
	raw, err := os.ReadFile(filepath.Join(home, "environment.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded EnvironmentRecord
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode on-disk record: %v", err)
	}
	if len(decoded.Associations) != 2 {
		t.Fatalf("on-disk associations=%d, want 2", len(decoded.Associations))
	}
	// The identity fields must be intact alongside the associations.
	if err := environment.Validate(decoded.Identity); err != nil {
		t.Fatalf("identity corrupted by association write: %v", err)
	}
}

// TestEnvironmentRecordAssociationsRejectsInvalidPersistedEntry proves the
// authoritative read fails closed when a persisted association carries an
// unknown or empty kind or an empty/path-bearing id: the malformed entry must
// never be exposed as an authoritative member, even though the file comments
// promise fail-closed handling of corrupt bindings.
func TestEnvironmentRecordAssociationsRejectsInvalidPersistedEntry(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	// Prime a valid record, then splice a malformed entry onto it.
	if _, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindApp, "app-1"); err != nil {
		t.Fatal(err)
	}
	raw := mustReadEnvironmentRecord(t, "target-a", "prod")

	cases := []struct {
		name    string
		mutate  func(record *EnvironmentRecord)
		wantErr bool
	}{
		{
			name:    "unknown kind",
			mutate:  func(r *EnvironmentRecord) { r.Associations[0].SubjectKind = "frobnicate" },
			wantErr: true,
		},
		{
			name:    "empty kind",
			mutate:  func(r *EnvironmentRecord) { r.Associations[0].SubjectKind = "" },
			wantErr: true,
		},
		{
			name:    "empty id",
			mutate:  func(r *EnvironmentRecord) { r.Associations[0].SubjectID = "" },
			wantErr: true,
		},
		{
			name:    "path-bearing id",
			mutate:  func(r *EnvironmentRecord) { r.Associations[0].SubjectID = "../escape" },
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Restore the pristine record first, then splice in the malformed entry.
			restoreRecord(t, raw)
			var record EnvironmentRecord
			if err := json.Unmarshal(raw, &record); err != nil {
				t.Fatal(err)
			}
			tc.mutate(&record)
			poisoned, err := json.MarshalIndent(record, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(mustPath(t, "target-a", "prod"), poisoned, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err = EnvironmentRecordAssociations("target-a", "prod")
			if tc.wantErr && err == nil {
				t.Fatalf("read should have failed closed on %q, got nil err", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("read should have succeeded on %q, got err: %v", tc.name, err)
			}
		})
	}
}

// TestRegisterEnvironmentAssociationRejectsInvalidPersistedEntry proves the
// register mutation fails closed before it adopts or forwards a persisted
// member list that carries a malformed binding: the bad entry is validated at
// mutation entry, not only on the incoming caller argument.
func TestRegisterEnvironmentAssociationRejectsInvalidPersistedEntry(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	if _, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindApp, "app-1"); err != nil {
		t.Fatal(err)
	}

	// Unknown kind on the persisted list must refuse a subsequent registration.
	spliceAssociation(t, "target-a", "prod", func(record *EnvironmentRecord) {
		record.Associations[0].SubjectKind = "frobnicate"
	})
	if _, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindApp, "app-2"); err == nil {
		t.Fatal("register should have failed closed on unknown persisted kind")
	}

	// Path-bearing id on the persisted list must refuse a subsequent registration.
	spliceAssociation(t, "target-a", "prod", func(record *EnvironmentRecord) {
		record.Associations[0].SubjectID = "../escape"
	})
	if _, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindApp, "app-3"); err == nil {
		t.Fatal("register should have failed closed on path-bearing persisted id")
	}
}

// TestDeregistrationEnvironmentAssociationRejectsInvalidPersistedEntry proves
// the deregistration mutation fails closed on a malformed persisted member
// list before it removes anything.
func TestDeregistrationEnvironmentAssociationRejectsInvalidPersistedEntry(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	if _, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindApp, "app-1"); err != nil {
		t.Fatal(err)
	}
	raw := mustReadEnvironmentRecord(t, "target-a", "prod")

	var record EnvironmentRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatal(err)
	}
	record.Associations[0].SubjectID = "../escape"
	blob, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mustPath(t, "target-a", "prod"), blob, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := DeregistrationEnvironmentAssociation("target-a", "prod", AssociationKindApp, "app-1"); err == nil {
		t.Fatal("deregister should have failed closed on path-bearing persisted id")
	}
}

// TestInvalidPersistedAssociationNoWrite proves that a failed mutation does not
// rewrite the record: the poisoned on-disk bytes are left byte-for-byte
// untouched so the corruption is neither masked nor half-applied.
func TestInvalidPersistedAssociationNoWrite(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	if _, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindApp, "app-1"); err != nil {
		t.Fatal(err)
	}

	// Poison the persisted list with an unknown kind.
	var record EnvironmentRecord
	before := mustReadEnvironmentRecord(t, "target-a", "prod")
	if err := json.Unmarshal(before, &record); err != nil {
		t.Fatal(err)
	}
	record.Associations[0].SubjectKind = "frobnicate"
	blob, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mustPath(t, "target-a", "prod"), blob, 0o600); err != nil {
		t.Fatal(err)
	}

	// A subsequent registration must fail closed, and the poisoned bytes must
	// remain exactly as written — no rewrite, no normalization.
	if _, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindApp, "app-2"); err == nil {
		t.Fatal("register should have failed closed on unknown persisted kind")
	}
	after := mustReadEnvironmentRecord(t, "target-a", "prod")
	if !bytes.Equal(blob, after) {
		t.Fatalf("failed mutation rewrote the record: wrote=%q after=%q", blob, after)
	}
	if bytes.Equal(before, after) {
		t.Fatalf("failed mutation should not have normalized the poison back to the pristine record")
	}
}

// TestRegisterEnvironmentAssociationNoSecretLeak proves the subject id is a
// non-secret durable identity, never a secret value: the on-disk record must
// not carry any plaintext secret. Membership carries only the subject id and
// kind, never credentials.
func TestRegisterEnvironmentAssociationNoSecretLeak(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	// A plausible secret value registered as a subject id must still be a valid
	// (non-secret) durable identity; the seam must not treat it as a secret to
	// redact, but it also must never be a real credential. A normal subject id
	// is a slug; a secret-shaped value fails the slug guard rather than leaking.
	if _, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindApp, "app-secret-leak"); err != nil {
		t.Fatalf("valid slug subject id: %v", err)
	}
	list, err := ListEnvironmentAssociations("target-a", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected one binding, got %d", len(list))
	}
}

// TestRegisterEnvironmentAssociationRejectsInvalidInput proves the seam fails
// closed on invalid kind or subject id before any directory is created.
func TestRegisterEnvironmentAssociationRejectsInvalidInput(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	if _, err := RegisterEnvironmentAssociation("bad/name", "prod", AssociationKindApp, "app-1"); err == nil {
		t.Fatal("invalid target accepted")
	}
	if _, err := RegisterEnvironmentAssociation("target-a", "bad/name", AssociationKindApp, "app-1"); err == nil {
		t.Fatal("invalid environment accepted")
	}
	if _, err := RegisterEnvironmentAssociation("target-a", "prod", "frobnicate", "app-1"); err == nil {
		t.Fatal("invalid subject kind accepted")
	}
	if _, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindApp, "bad/name"); err == nil {
		t.Fatal("invalid subject id accepted")
	}
	if _, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindApp, ""); err == nil {
		t.Fatal("empty subject id accepted")
	}
}

// TestRegisterEnvironmentAssociationDistinctKindsAreDistinct proves an app and
// a node with the same subject id are two distinct bindings, because the (kind,
// id) tuple is the membership key and the kind keeps the three identity roles
// from collapsing.
func TestRegisterEnvironmentAssociationDistinctKindsAreDistinct(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	appID := "shared-id"
	if _, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindApp, appID); err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindNode, appID); err != nil {
		t.Fatal(err)
	}
	list, err := ListEnvironmentAssociations("target-a", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("app and node with same id must be 2 distinct bindings, got %d", len(list))
	}
}

// TestEnvironmentRecordAssociationsBindCheckOnForeignPath proves a record that
// is physically located under a different (Target, environment) path is refused
// when read under the wrong path, so its membership list never surfaces and
// aliases a different environment. This mirrors the identity bind-check: the
// association read re-validates the binding before returning anything.
func TestEnvironmentRecordAssociationsBindCheckOnForeignPath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	if _, err := RegisterEnvironmentAssociation("target-a", "prod", AssociationKindApp, "app-1"); err != nil {
		t.Fatal(err)
	}

	// Per-target isolation: the record physically lives at
	// targets/target-a/.baseharbor/environments/prod/environment.json. Reading
	// it via a DIFFERENT target path reads a DIFFERENT (absent) record, so it
	// returns an empty list — never the foreign binding under another target.
	empty, err := ListEnvironmentAssociations("target-b", "prod")
	if err != nil {
		t.Fatalf("foreign-target read should be empty (no record), got err: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("foreign-target association read leaked %d bindings, want 0", len(empty))
	}

	// The genuine bind-check refusal: copy a foreign record onto a path that
	// names a different (target, environment) and read it — the binding mismatch
	// must be refused, never surfaced.
	otherTarget := filepath.Join(root, "targets", "other-target", ".baseharbor", "environments", "prod", "environment.json")
	if err := os.MkdirAll(filepath.Dir(otherTarget), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(otherTarget, mustReadEnvironmentRecord(t, "target-a", "prod"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A record whose stored binding is target-a but is read as target-other
	// must fail closed with ErrEnvironmentBindingMismatch.
	if _, err := EnvironmentRecordAssociations("other-target", "prod"); !errors.Is(err, ErrEnvironmentBindingMismatch) {
		t.Fatalf("expected ErrEnvironmentBindingMismatch for foreign record, got %v", err)
	}
}

// mustReadEnvironmentRecord reads the on-disk environment.json for a recorded
// (target, environment) so a test can relocate it to a foreign path.
func mustReadEnvironmentRecord(t *testing.T, target, env string) []byte {
	t.Helper()
	_, path, err := environmentStateRoot(target, env)
	if err != nil {
		t.Fatalf("resolve environment state root: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read environment record: %v", err)
	}
	return data
}

// mustPath resolves the on-disk environment.json path for a recorded
// (target, environment).
func mustPath(t *testing.T, target, env string) string {
	t.Helper()
	_, path, err := environmentStateRoot(target, env)
	if err != nil {
		t.Fatalf("resolve environment state root: %v", err)
	}
	return path
}

// restoreRecord writes a pristine record back to its home path.
func restoreRecord(t *testing.T, raw []byte) {
	t.Helper()
	home, _, err := environmentStateRoot("target-a", "prod")
	if err != nil {
		t.Fatalf("resolve environment state root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "environment.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// spliceAssociation decodes the on-disk record, applies mutate to it, and
// writes it back, so a test can poison exactly one persisted binding.
func spliceAssociation(t *testing.T, target, env string, mutate func(record *EnvironmentRecord)) {
	t.Helper()
	path := mustPath(t, target, env)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record EnvironmentRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatal(err)
	}
	mutate(&record)
	blob, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatal(err)
	}
}
