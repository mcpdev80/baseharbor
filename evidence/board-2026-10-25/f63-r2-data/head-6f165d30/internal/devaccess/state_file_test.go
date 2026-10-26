package devaccess

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeveloperStateReplacementNeverFollowsPredictableTemporaryFile(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	if _, err := Configure("isolated", "dev", "developer", "original"); err != nil {
		t.Fatal(err)
	}
	path, _ := Path("isolated", "dev")
	foreign := filepath.Join(t.TempDir(), "foreign")
	if err := os.WriteFile(foreign, []byte("foreign"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, path+".tmp"); err != nil {
		t.Fatal(err)
	}
	if _, err := Configure("isolated", "dev", "changed", "replacement"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(foreign); err != nil || string(data) != "foreign" {
		t.Fatal("replacement changed foreign material through a stale temporary file")
	}
	loaded, err := Load("isolated", "dev")
	if err != nil || loaded.Username != "changed" || loaded.Password != "replacement" {
		t.Fatal("explicit replacement did not converge to protected authoritative state")
	}
	if matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".developer-state-*")); err != nil || len(matches) != 0 {
		t.Fatal("replacement left credential-bearing temporary material")
	}
}

func TestDeveloperStateRejectsRedirectedParentsOnReadAndWrite(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	path, _ := Path("isolated", "dev")
	foreign := t.TempDir()
	parent := filepath.Dir(path)
	if err := os.MkdirAll(filepath.Dir(parent), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, parent); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreign, "credentials.env"), []byte("USERNAME=foreign\nPASSWORD=private\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("isolated", "dev"); err == nil {
		t.Fatal("read accepted redirected credential authority")
	}
	if _, err := Configure("isolated", "dev", "developer", "replacement"); err == nil {
		t.Fatal("write accepted redirected credential authority")
	}
	if _, err := ConfigureDomain("isolated", "changed.localhost"); err == nil {
		t.Fatal("domain mutation followed a redirected parent")
	}
	if data, err := os.ReadFile(filepath.Join(foreign, "credentials.env")); err != nil || string(data) != "USERNAME=foreign\nPASSWORD=private\n" {
		t.Fatal("foreign credential recovery material changed")
	}
}

func TestDeveloperStatePreservesDamagedOrUnprotectedMaterial(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	path, _ := Path("isolated", "dev")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"USERNAME=developer\nPASSWORD=one\nPASSWORD=two\n", "USERNAME=developer\nPASSWORD=one\nUNKNOWN=value\n", strings.Repeat("x", maxDeveloperStateBytes+1)} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Ensure("isolated", "dev"); err == nil {
			t.Fatal("initialization adopted or replaced ambiguous credential state")
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != data {
			t.Fatal("failed initialization changed recovery material")
		}
	}
	if err := os.WriteFile(path, []byte("USERNAME=developer\nPASSWORD=one\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Configure("isolated", "dev", "developer", "replacement"); err == nil {
		t.Fatal("replacement overwrote unprotected authority")
	}
}
