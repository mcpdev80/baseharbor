package application

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestManifestRoundTrip(t *testing.T) {
	want := New("mailflow", "prod", true, true, true)
	want.Secrets.Required = []string{"SMTP_PASSWORD", "OPENAI_API_KEY"}
	got, err := ParseYAML(want.YAML())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, Manifest{Version: want.Version, Name: want.Name, Environment: want.Environment, Services: want.Services, Secrets: SecretRequirements{Required: []string{"OPENAI_API_KEY", "SMTP_PASSWORD"}}}) {
		t.Fatalf("round trip mismatch: got %#v want %#v", got, want)
	}
}

func TestManifestDefaultsToPostgres(t *testing.T) {
	m := New("demo", "", false, false, false)
	if m.Environment != "dev" || !m.Services.Postgres {
		t.Fatalf("unexpected defaults: %#v", m)
	}
}

func TestManifestValidationFailsClosed(t *testing.T) {
	cases := []Manifest{
		New("UPPER", "dev", true, false, false),
		{Version: 99, Name: "demo", Environment: "dev", Services: Services{Postgres: true}},
		{Version: 1, Name: "demo", Environment: "dev"},
		{Version: 1, Name: "demo", Environment: "dev", Services: Services{Postgres: true}, Secrets: SecretRequirements{Required: []string{"API_TOKEN"}}},
		{Version: 1, Name: "demo", Environment: "dev", Services: Services{Secrets: true}, Secrets: SecretRequirements{Required: []string{"bad/key"}}},
		{Version: 1, Name: "demo", Environment: "dev", Services: Services{Secrets: true}, Secrets: SecretRequirements{Required: []string{"API_TOKEN", "API_TOKEN"}}},
	}
	for _, tc := range cases {
		if err := tc.Validate(); err == nil {
			t.Fatalf("expected validation failure for %#v", tc)
		}
	}
}

func TestStoreCreateLoadList(t *testing.T) {
	root := filepath.Join(t.TempDir(), "apps")
	store := Store{Root: root}
	for _, name := range []string{"zeta", "alpha"} {
		path, err := store.Create(New(name, "dev", true, false, false))
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("manifest permissions = %o", info.Mode().Perm())
		}
	}
	m, _, err := store.Load("alpha")
	if err != nil || m.Name != "alpha" {
		t.Fatalf("load failed: %#v %v", m, err)
	}
	items, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Name != "alpha" || items[1].Name != "zeta" {
		t.Fatalf("unexpected list: %#v", items)
	}
	_, err = store.Create(New("alpha", "dev", true, false, false))
	if !errors.Is(err, ErrExists) {
		t.Fatalf("expected ErrExists, got %v", err)
	}
}

func TestParserRejectsUnknownFields(t *testing.T) {
	_, err := ParseYAML(strings.Replace(New("demo", "dev", true, false, false).YAML(), "app:\n", "app:\n  surprise: yes\n", 1))
	if err == nil {
		t.Fatal("expected unknown field to fail")
	}
}
