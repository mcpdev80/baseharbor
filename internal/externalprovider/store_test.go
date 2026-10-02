package externalprovider

import (
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestStoreRegisterListRemove(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "external-providers.json")}
	reg := Registration{
		ID: "company-db", ProviderID: "company/postgresql", Endpoint: "postgres://db.example:5432/app",
		CredentialRef: "vault://team/app/db",
		Provider:      capability.Provider{Kind: capability.ProviderKind("company-postgresql"), Capabilities: []capability.Kind{capability.SQL}},
		Trust:         Trust{Mode: TrustSystem},
	}
	if err := store.Register(reg); err != nil {
		t.Fatal(err)
	}
	items, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "company-db" {
		t.Fatalf("items=%#v", items)
	}
	if err := store.Remove("company-db"); err != nil {
		t.Fatal(err)
	}
	items, err = store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("items after remove=%#v", items)
	}
}
