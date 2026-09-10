package application

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveServiceBindingDefaultPostgres(t *testing.T) {
	root := t.TempDir()
	files := RuntimeFiles{Bindings: filepath.Join(root, "bindings")}
	dir := filepath.Join(files.Bindings, "postgres")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"host": "127.0.0.1\n", "port": "15432\n", "database": "demo_dev\n", "username": "baseharbor\n", "password": "secret\n", "uri": "postgresql://baseharbor:secret@127.0.0.1:15432/demo_dev\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	binding, err := ResolveServiceBinding(files, "postgres", "default")
	if err != nil {
		t.Fatal(err)
	}
	if binding.Host != "127.0.0.1" || binding.Port != "15432" || binding.Database != "demo_dev" || binding.Password != "secret" {
		t.Fatalf("unexpected binding: %#v", binding)
	}
}

func TestResolveServiceBindingNamedValkey(t *testing.T) {
	root := t.TempDir()
	files := RuntimeFiles{Bindings: filepath.Join(root, "bindings")}
	dir := filepath.Join(files.Bindings, "valkey", "sessions")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"host": "127.0.0.1", "port": "16379", "password": "secret", "uri": "redis://:secret@127.0.0.1:16379/0"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	binding, err := ResolveServiceBinding(files, "valkey", "sessions")
	if err != nil {
		t.Fatal(err)
	}
	if binding.Instance != "sessions" || binding.Port != "16379" || binding.Username != "" {
		t.Fatalf("unexpected binding: %#v", binding)
	}
}

func TestResolveServiceBindingMissingFailsClosed(t *testing.T) {
	_, err := ResolveServiceBinding(RuntimeFiles{Bindings: t.TempDir()}, "postgres", "default")
	if err == nil {
		t.Fatal("expected missing materialized binding to fail")
	}
}
