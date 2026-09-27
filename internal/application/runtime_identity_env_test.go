package application

import (
	"strings"
	"testing"
)

func TestRuntimeEnvContentPreservesManagedIdentityValues(t *testing.T) {
	m := New("demo", "dev", false, false, false)
	m.Services.SQL = false
	m.Services.Identity = true
	m.Identity.Scopes = []string{"openid", "profile"}

	values := map[string]string{
		"IDENTITY_CONTAINER_ISSUER": "https://identity.localhost:18443/realms/bh-demo-dev",
		"IDENTITY_CLIENT_ID":        "baseharbor-demo-dev",
		"IDENTITY_CLIENT_SECRET":    "secret-value",
		"IDENTITY_CA_FILE":          "/tmp/baseharbor/identity/ca.crt",
	}

	rendered := runtimeEnvContent(m, values)
	for key, value := range values {
		want := key + "=" + value + "\n"
		if !strings.Contains(rendered, want) {
			t.Fatalf("identity runtime value %s was dropped:\n%s", key, rendered)
		}
	}
}
