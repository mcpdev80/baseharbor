package application

import (
	"path/filepath"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func TestSharedBackendSelectiveDependencyWithoutManagementBundle(t *testing.T) {
	issuer, err := serviceaccess.EnsureLocalIssuer(filepath.Join(t.TempDir(), "pki"))
	if err != nil {
		t.Fatal(err)
	}
	for _, services := range []struct{ sql, cache bool }{{true, false}, {false, true}, {true, true}} {
		m := New("selective", "dev", services.sql, services.cache, false)
		state := sharedBackendState{Applications: map[string]sharedBackendAppState{}}
		core, err := selectSharedBackendDependency(&state, m, issuer, "local")
		if err != nil || core.Project != "" || state.CoreSQL != nil {
			t.Fatalf("implicit management SQL: %+v, %v", state, err)
		}
		if services.sql && state.PostgresMembers != 1 {
			t.Fatal("standalone SQL topology not resolved")
		}
	}
}

func TestSharedBackendSelectiveDependencyPreservesExistingBinding(t *testing.T) {
	issuer, err := serviceaccess.EnsureLocalIssuer(filepath.Join(t.TempDir(), "pki"))
	if err != nil {
		t.Fatal(err)
	}
	state := sharedBackendState{CoreSQL: &bhruntime.Files{Project: "existing-management"}}
	if _, err := selectSharedBackendDependency(&state, New("selective", "dev", true, false, false), issuer, "local"); err == nil {
		t.Fatal("existing SQL provider implicitly migrated")
	}
	if state.CoreSQL.Project != "existing-management" {
		t.Fatal("existing provider binding overwritten")
	}
}
