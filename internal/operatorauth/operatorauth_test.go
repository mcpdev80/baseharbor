package operatorauth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigValidateRejectsInsecureOrIncompleteOIDC(t *testing.T) {
	cases := []Config{
		{Issuer: "http://id.example.test", ClientID: "baha"},
		{Issuer: "https://id.example.test", ClientID: ""},
		{Issuer: "https://id.example.test?token=x", ClientID: "baha"},
		{Issuer: "https://id.example.test", ClientID: "baha", Scopes: []string{"openid profile"}},
	}
	for _, cfg := range cases {
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected invalid config to fail: %#v", cfg)
		}
	}
}

func TestManagedEnvironmentKeepsDevTrustedLocal(t *testing.T) {
	if ManagedEnvironment("dev") || ManagedEnvironment("development") {
		t.Fatal("development must stay trusted-local")
	}
	if !ManagedEnvironment("test") || !ManagedEnvironment("prod") {
		t.Fatal("test/prod must require managed operator authentication")
	}
}

func TestSessionIsTargetAndEnvironmentScopedAndExpires(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	session := Session{
		Target:      "local",
		Environment: "test",
		Issuer:      "https://id.example.test",
		Subject:     "operator-1",
		ClientID:    "baseharbor-cli",
		IDToken:     "opaque-test-token",
		ExpiresAt:   time.Now().Add(5 * time.Minute),
	}
	if err := SaveSession(session); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(runtimeDir, "baseharbor", "operator-local-test.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("session mode=%#o", info.Mode().Perm())
	}
	if _, err := LoadSession("local", "prod"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cross-environment session unexpectedly resolved: %v", err)
	}
	if _, err := LoadSession("other", "test"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cross-target session unexpectedly resolved: %v", err)
	}

	cfg := Config{Issuer: session.Issuer, ClientID: session.ClientID}
	observed, err := ObserveSession("local", "test", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !observed.Present || !observed.Valid || observed.Subject != session.Subject {
		t.Fatalf("observation=%#v", observed)
	}

	session.ExpiresAt = time.Now().Add(10 * time.Second)
	if err := SaveSession(session); err == nil {
		t.Fatal("near-expired session must not be persisted")
	}
}
