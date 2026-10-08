package coreupdate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/providerbinding"
	"github.com/mcpdev80/baseharbor/internal/providerupgrade"
)

type mockBoundAdapter struct{ backupCalls, applies, verifies int }

func (a *mockBoundAdapter) Inventory(context.Context) (providerupgrade.Inventory, error) {
	return providerupgrade.Inventory{Provider: providerupgrade.ProviderOpenBao, Owner: "baseharbor", Healthy: true, Version: "2.7.0"}, nil
}
func (a *mockBoundAdapter) Preflight(context.Context, providerupgrade.Request) (providerupgrade.Assessment, error) {
	return providerupgrade.Assessment{Classification: providerupgrade.ClassificationSupported, BackupRequired: true}, nil
}
func (a *mockBoundAdapter) Backup(context.Context, providerupgrade.Request) (providerupgrade.BackupRef, error) {
	a.backupCalls++
	return providerupgrade.BackupRef{Provider: providerupgrade.ProviderOpenBao, ID: "immutable-ref", Version: "2.7.0", Verified: true, CreatedAt: time.Now().UTC()}, nil
}
func (a *mockBoundAdapter) Execute(_ context.Context, _ providerupgrade.Request, backup providerupgrade.BackupRef) error {
	a.applies++
	if backup.ID != "immutable-ref" {
		panic("unverified recovery ref")
	}
	return nil
}
func (a *mockBoundAdapter) Verify(context.Context, providerupgrade.Request) error {
	a.verifies++
	return nil
}
func (a *mockBoundAdapter) Recover(context.Context, providerupgrade.Request, providerupgrade.BackupRef) error {
	return nil
}

type mockBoundRegistry struct{ adapter *mockBoundAdapter }

func (r mockBoundRegistry) Resolve(_ context.Context, _ providerupgrade.Provider) (providerupgrade.Adapter, providerbinding.RuntimeIdentity, error) {
	return r.adapter, providerbinding.RuntimeIdentity{Owned: true}, nil
}
func (r mockBoundRegistry) Preflight(ctx context.Context, p providerupgrade.Provider, req providerupgrade.Request) (providerupgrade.Adapter, providerupgrade.Assessment, error) {
	assessment, err := r.adapter.Preflight(ctx, req)
	return r.adapter, assessment, err
}
func TestBoundOpenBaoUsesDurableCoreJournal(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	a := &mockBoundAdapter{}
	txn := BoundProviderTransaction{Registry: mockBoundRegistry{adapter: a}, BackupDirectory: dir, RecordExternal: func(context.Context, Delta, string) error { return nil }}
	delta := Delta{Installed: Realization{Kind: Secrets, Installation: "core", Scope: "shared", Instance: "openbao", Owner: "baseharbor", Version: "2.7.0", Image: "openbao:old", Digest: digestA}, Desired: Desired{Kind: Secrets, Version: "2.7.1", Image: "openbao:new", Digest: digestB}, Classification: BackupRequired}
	plan := Plan{Release: "0.4.24", Deltas: []Delta{delta}}
	journal := filepath.Join(dir, "journal.json")
	if err := ExecuteJournaled(context.Background(), plan, journal, txn.Hooks()); err != nil {
		t.Fatal(err)
	}
	if a.applies != 1 || a.backupCalls != 1 || a.verifies != 1 {
		t.Fatalf("provider hooks not journaled: %+v", a)
	}
	ref, err := txn.loadBackup(delta)
	if err != nil || ref.ID != "immutable-ref" {
		t.Fatalf("durable backup unavailable: %+v %v", ref, err)
	}
	if err := ExecuteJournaled(context.Background(), plan, journal, txn.Hooks()); err != nil {
		t.Fatal(err)
	}
	if a.applies != 1 || a.backupCalls != 1 || a.verifies != 2 {
		t.Fatalf("verified provider was restarted: %+v", a)
	}
	if err := os.WriteFile(filepath.Join(dir, "tamper"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestBoundProviderRejectsMissingHooksAndBackup(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	delta := Delta{Installed: Realization{Kind: Identity, Installation: "core", Scope: "shared", Instance: "keycloak", Owner: "baseharbor", Version: "26.8.0"}, Desired: Desired{Kind: Identity, Version: "26.8.1", Image: "keycloak:new", Digest: digestB}, Classification: MigrationRequired}
	txn := BoundProviderTransaction{BackupDirectory: dir}
	if err := txn.Hooks().Preflight(context.Background(), Plan{Release: "0.4.24", Deltas: []Delta{delta}}); err == nil {
		t.Fatal("missing native registry accepted")
	}
	txn.Registry = mockBoundRegistry{adapter: &mockBoundAdapter{}}
	txn.RecordExternal = func(context.Context, Delta, string) error { return nil }
	if err := txn.Hooks().Apply(context.Background(), delta); err == nil {
		t.Fatal("mutation accepted without verified durable backup")
	}
}
