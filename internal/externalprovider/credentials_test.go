package externalprovider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveCredentialReferenceRequiresOwnerOnlyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte("{\"username\":\"app\",\"password\":\"secret\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	creds, err := ResolveCredentialReference("file://" + path)
	if err != nil {
		t.Fatal(err)
	}
	if creds.Username != "app" || creds.Password != "secret" {
		t.Fatalf("credentials=%#v", creds)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveCredentialReference("file://" + path); err == nil {
		t.Fatal("expected group/world-readable credential reference to fail")
	}
}
