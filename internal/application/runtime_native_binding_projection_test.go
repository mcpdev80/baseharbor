package application

import (
	"net/url"
	"reflect"
	"strconv"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestNativeCacheProjectionPreservesSharedCredentialOwnerAndUsesActualPrincipal(t *testing.T) {
	m := New("owned", "dev", false, true, false)
	retained := map[string]string{"VALKEY_USER": "shared_owner", "VALKEY_PASSWORD": "retained-test-reference-value", "VALKEY_KEY_PREFIX": "owned-prefix:", "VALKEY_HOST_PORT": "16379"}
	before := map[string]string{}
	for key, value := range retained {
		before[key] = value
	}
	for _, scope := range []capability.ProviderScope{capability.ScopeApplication, capability.ScopeShared, capability.ScopeApplication} {
		t.Setenv(ProviderScopeEnv(capability.ProviderValkey), string(scope))
		projected := nativeRuntimeBindingValues(m, retained)
		username := "shared_owner"
		if scope == capability.ScopeApplication {
			username = "default"
		}
		if projected["VALKEY_USER"] != username || projected["VALKEY_PASSWORD"] != retained["VALKEY_PASSWORD"] || projected["VALKEY_KEY_PREFIX"] != retained["VALKEY_KEY_PREFIX"] {
			t.Fatal("binding changed retained password/namespace or assumed wrong native ACL role")
		}
		environment, err := containerRuntimeEnvironment(m, retained)
		if err != nil {
			t.Fatal(err)
		}
		uri, err := url.Parse(environment["REDIS_URL"])
		if err != nil || uri.User.Username() != username {
			t.Fatal("workload binding differs from actual provider authentication principal")
		}
		if !reflect.DeepEqual(before, retained) {
			t.Fatal("native binding projection rewrote original shared credentials")
		}
	}
}

func TestNativeEndpointOwnerSurvivesSharedBindingAndAvoidsHandoffPortCollision(t *testing.T) {
	m := New("owned", "dev", true, true, false)
	values := map[string]string{"POSTGRES_HOST_PORT": "15432", "VALKEY_HOST_PORT": "16379"}
	for _, kind := range []capability.ProviderKind{capability.ProviderPostgreSQL, capability.ProviderValkey} {
		t.Setenv(ProviderScopeEnv(kind), "shared")
	}
	if err := ensureNativeBackendEndpointValues(values, m); err != nil {
		t.Fatal(err)
	}
	values["POSTGRES_HOST_PORT"], values["VALKEY_HOST_PORT"] = "25432", "26379"
	values["POSTGRES_CONTAINER_HOST"], values["VALKEY_CONTAINER_HOST"] = "shared-sql", "shared-cache"
	shared := nativeRuntimeBindingValues(m, values)
	if shared["POSTGRES_CONTAINER_HOST"] != "shared-sql" || shared["VALKEY_CONTAINER_HOST"] != "shared-cache" {
		t.Fatal("shared destination binding changed")
	}
	for _, kind := range []capability.ProviderKind{capability.ProviderPostgreSQL, capability.ProviderValkey} {
		t.Setenv(ProviderScopeEnv(kind), "application")
	}
	if err := ensureNativeBackendEndpointValues(values, m); err != nil {
		t.Fatal(err)
	}
	if values["POSTGRES_HOST_PORT"] != "15432" || values["VALKEY_HOST_PORT"] != "16379" || values["POSTGRES_CONTAINER_HOST"] != "postgres" || values["VALKEY_CONTAINER_HOST"] != "valkey-access" {
		t.Fatal("native endpoint adopted shared host/port or lost its retained port owner")
	}
	serialized, err := readRuntimeEnvBytes([]byte(runtimeEnvContent(m, values)))
	if err != nil || serialized["POSTGRES_APPLICATION_HOST_PORT"] != "15432" || serialized["VALKEY_APPLICATION_HOST_PORT"] != "16379" {
		t.Fatal("existing protected runtime owner did not retain native ports")
	}
	// Existing shared owners without a prior native port receive distinct ports.
	values = map[string]string{"POSTGRES_HOST_PORT": "25432", "VALKEY_HOST_PORT": "26379", "POSTGRES_CONTAINER_HOST": "shared-sql", "VALKEY_CONTAINER_HOST": "shared-cache"}
	if err := ensureNativeBackendEndpointValues(values, m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"POSTGRES_HOST_PORT", "VALKEY_HOST_PORT"} {
		port, err := strconv.Atoi(values[key])
		if err != nil || port <= 0 || port == 25432 || port == 26379 {
			t.Fatal("native handoff port is invalid or collides with an existing shared port")
		}
	}
	before := values["POSTGRES_HOST_PORT"]
	if err := ensureNativeBackendEndpointValues(values, m); err != nil || values["POSTGRES_HOST_PORT"] != before {
		t.Fatal("repeated native realization changed its port owner")
	}
}
