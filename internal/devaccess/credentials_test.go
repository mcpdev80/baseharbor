package devaccess

import (
	"os"
	"testing"
)

func TestDeveloperCredentialsAreTargetScopedAndUsernameConfigurable(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	first, err := Ensure("local", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if first.Username != DefaultUsername {
		t.Fatalf("default username = %q, want %q", first.Username, DefaultUsername)
	}

	configured, err := Configure("local", "dev", "marcel", "custom-secret")
	if err != nil {
		t.Fatal(err)
	}
	if configured.Username != "marcel" {
		t.Fatalf("configured username = %q", configured.Username)
	}

	reused, err := Ensure("local", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if reused != configured {
		t.Fatalf("Ensure created new credentials instead of reusing target credentials: got %#v want %#v", reused, configured)
	}

	path, err := Path("local", "dev")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("credential permissions = %o, want 600", info.Mode().Perm())
	}
}
