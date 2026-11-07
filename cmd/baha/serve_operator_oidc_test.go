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

func TestWorkstationEnrollmentCannotSkipOwnedOIDCBoundary(t *testing.T) {
	for _, cfg := range []controlplaneruntime.Config{
		{WorkstationEnrollmentEnabled: true},
		{WorkstationEnrollmentEnabled: true, OIDCIssuer: "https://foreign.example/realms/other"},
	} {
		if err := bindServeToOwnedOperatorOIDC(&cfg); err == nil {
			t.Fatalf("workstation enrollment bypassed verified owner OIDC boundary: %#v", cfg)
		}
	}
}

func TestServeEnrollmentAuthorityTargetBindsBothEnrollmentModes(t *testing.T) {
	cases := []struct {
		name string
		cfg  controlplaneruntime.Config
		want string
	}{
		{name: "connector", cfg: controlplaneruntime.Config{ConnectorEnrollmentEnabled: true, ConnectorAuthorityTarget: " owner "}, want: "owner"},
		{name: "workstation", cfg: controlplaneruntime.Config{WorkstationEnrollmentEnabled: true, ConnectorAuthorityTarget: " owner "}, want: "owner"},
		{name: "both", cfg: controlplaneruntime.Config{ConnectorEnrollmentEnabled: true, WorkstationEnrollmentEnabled: true, ConnectorAuthorityTarget: "owner"}, want: "owner"},
		{name: "neither", cfg: controlplaneruntime.Config{ConnectorAuthorityTarget: "owner"}, want: ""},
		{name: "broker", cfg: controlplaneruntime.Config{RuntimeAppName: "app", WorkstationEnrollmentEnabled: true, ConnectorAuthorityTarget: "owner"}, want: ""},
		{name: "missing authority", cfg: controlplaneruntime.Config{WorkstationEnrollmentEnabled: true}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := serveEnrollmentAuthorityTarget(tc.cfg); got != tc.want {
				t.Fatalf("enrollment authority target = %q, want %q", got, tc.want)
			}
		})
	}
}
