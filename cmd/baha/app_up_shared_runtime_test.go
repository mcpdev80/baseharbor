package main

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestShouldStartApplicationManagedRuntimeSkipsDefaultSharedBackends(t *testing.T) {
	m := application.New("demo", "dev", true, true, true)
	if !application.HasManagedRuntimeServices(m) {
		t.Fatal("test fixture must contain managed SQL/cache capabilities")
	}
	if shouldStartApplicationManagedRuntime(m) {
		t.Fatal("default shared PostgreSQL/Valkey must not start an application-scoped compose project")
	}
}

func TestShouldStartApplicationManagedRuntimeStartsApplicationScopedSQL(t *testing.T) {
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication))
	m := application.New("demo", "dev", true, false, false)
	if !shouldStartApplicationManagedRuntime(m) {
		t.Fatal("application-scoped PostgreSQL must start the application runtime project")
	}
}
