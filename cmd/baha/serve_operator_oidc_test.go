package main

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/controlplaneruntime"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

func TestCoreConnectorOIDCUsesConfiguredOperatorRealm(t *testing.T) {
	operator := operatorauth.Config{Provider: "managed-keycloak", Issuer: "https://keycloak.example/realms/bh-operators-dev", ClientID: "baseharbor-cli-dev", Scopes: []string{"openid"}}
	cfg := controlplaneruntime.Config{}
	if err := matchOwnedOperatorOIDC(&cfg, operator); err != nil {
		t.Fatal(err)
	}
	if cfg.OIDCIssuer != operator.Issuer || len(cfg.OIDCAudiences) != 1 || cfg.OIDCAudiences[0] != operator.ClientID {
		t.Fatalf("Core not bound to operator realm/client: %#v", cfg)
	}
	for _, bad := range []controlplaneruntime.Config{
		{OIDCIssuer: "https://keycloak.example/realms/baseharbor"},
		{OIDCAudiences: []string{"unexpected-client"}},
	} {
		if err := matchOwnedOperatorOIDC(&bad, operator); err == nil {
			t.Fatalf("mismatched operator OIDC was accepted: %#v", bad)
		}
	}
	cfg = controlplaneruntime.Config{OIDCIssuer: operator.Issuer, OIDCAudiences: []string{"automation-client", operator.ClientID}}
	if err := matchOwnedOperatorOIDC(&cfg, operator); err != nil {
		t.Fatalf("matching explicit client was rejected: %v", err)
	}
}
