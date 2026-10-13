package application

import (
	"net/url"
	"reflect"
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
