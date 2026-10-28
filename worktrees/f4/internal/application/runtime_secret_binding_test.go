package application

import (
	"path/filepath"
	"testing"
)

func TestRequiredSecretFileBindingConvention(t *testing.T) {
	m := WithRequiredSecrets(New("demo", "prod", true, false, true), "API_TOKEN", "TLS_CERT_FILE", "TLS_KEY_FILE")
	if !HasRequiredFileSecrets(m) {
		t.Fatal("expected *_FILE required secrets to enable file bindings")
	}
	if RequiredSecretUsesFileBinding("API_TOKEN") {
		t.Fatal("ordinary secret unexpectedly uses a file binding")
	}
	if !RequiredSecretUsesFileBinding("TLS_KEY_FILE") {
		t.Fatal("expected TLS_KEY_FILE to use a file binding")
	}
	files := RuntimeFiles{Bindings: filepath.Join("state", "bindings")}
	if got, want := SecretFileHostPath(files, "TLS_KEY_FILE"), filepath.Join("state", "bindings", "secrets", "TLS_KEY_FILE"); got != want {
		t.Fatalf("unexpected host binding path %q want %q", got, want)
	}
	if got, want := SecretFileContainerPath("TLS_KEY_FILE"), "/run/baseharbor/bindings/secrets/TLS_KEY_FILE"; got != want {
		t.Fatalf("unexpected container binding path %q want %q", got, want)
	}
}
