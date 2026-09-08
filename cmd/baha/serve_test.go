package main

import (
	"testing"
)

func TestSplitNonEmpty(t *testing.T) {
	got := splitNonEmpty(" api-a, ,api-b ,api-a ")
	want := []string{"api-a", "api-b", "api-a"}
	if len(got) != len(want) {
		t.Fatalf("splitNonEmpty() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("splitNonEmpty()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestControlPlaneConfigFromEnvAllowsRuntimeOnlyMode(t *testing.T) {
	t.Setenv("BASEHARBOR_API_DATABASE_URL", "")
	t.Setenv("BASEHARBOR_API_OIDC_ISSUER", "")
	t.Setenv("BASEHARBOR_API_OIDC_AUDIENCES", "")
	t.Setenv("BASEHARBOR_API_TLS_CERT_FILE", "/run/baseharbor/tls.crt")
	t.Setenv("BASEHARBOR_API_TLS_KEY_FILE", "/run/baseharbor/tls.key")
	cfg, err := controlPlaneConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "" || cfg.OIDCIssuer != "" || len(cfg.OIDCAudiences) != 0 {
		t.Fatalf("unexpected runtime-only config: %#v", cfg)
	}
}

func TestControlPlaneConfigFromEnv(t *testing.T) {
	t.Setenv("BASEHARBOR_API_LISTEN_ADDR", "127.0.0.1:9443")
	t.Setenv("BASEHARBOR_API_DATABASE_URL", "postgres://runtime@example/baseharbor")
	t.Setenv("BASEHARBOR_API_OIDC_ISSUER", "https://issuer.example")
	t.Setenv("BASEHARBOR_API_OIDC_AUDIENCES", "control-plane,automation")
	t.Setenv("BASEHARBOR_API_TLS_CERT_FILE", "/run/baseharbor/tls.crt")
	t.Setenv("BASEHARBOR_API_TLS_KEY_FILE", "/run/baseharbor/tls.key")

	cfg, err := controlPlaneConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "127.0.0.1:9443" || cfg.OIDCIssuer != "https://issuer.example" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if len(cfg.OIDCAudiences) != 2 || cfg.OIDCAudiences[0] != "control-plane" || cfg.OIDCAudiences[1] != "automation" {
		t.Fatalf("unexpected audiences: %#v", cfg.OIDCAudiences)
	}
}
