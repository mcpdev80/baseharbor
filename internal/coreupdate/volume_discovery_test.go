package coreupdate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveOwnedServiceVolumeStrict(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "compose.yaml")
	data := "services:\n  postgres-member-1:\n    image: postgres:18\n    volumes:\n      - postgres-data-1:/var/lib/postgresql\n      - ./tls:/run/tls:ro\n  keycloak-1:\n    image: keycloak:26\n    volumes:\n      - ./tls:/run/tls:ro\nvolumes:\n  postgres-data-1: {}\n"
	if err := os.WriteFile(p, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	volume, err := ResolveOwnedServiceVolume(p, "postgres-member-1", "owned-p")
	if err != nil || volume != "owned-p_postgres-data-1" {
		t.Fatalf("unexpected volume %q: %v", volume, err)
	}
	if _, err := ResolveOwnedServiceVolume(p, "keycloak-1", "owned-p"); err == nil {
		t.Fatal("Keycloak's TLS-only volume is not database recovery")
	}
	if _, err := ResolveOwnedServiceVolume(p, "foreign", "owned-p"); err == nil {
		t.Fatal("unknown provider accepted")
	}
	bind := strings.Replace(data, "postgres-data-1:/var/lib/postgresql", "./postgres:/var/lib/postgresql", 1)
	if err := os.WriteFile(p, []byte(bind), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveOwnedServiceVolume(p, "postgres-member-1", "owned-p"); err == nil {
		t.Fatal("bind-backed SQL accepted without native backup")
	}
}

func TestResolveOwnedVolumeHonorsExplicitNameButRejectsExternal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compose.yaml")
	raw := "services:\n  openbao-member-1:\n    volumes:\n      - bao-data:/openbao/file\nvolumes:\n  bao-data:\n    name: protected-bao-volume\n"
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	volume, err := ResolveOwnedServiceVolume(path, "openbao-member-1", "baseharbor")
	if err != nil || volume != "protected-bao-volume" {
		t.Fatalf("wrong declared volume %q %v", volume, err)
	}
	external := strings.Replace(raw, "name: protected-bao-volume", "external: true", 1)
	if err := os.WriteFile(path, []byte(external), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveOwnedServiceVolume(path, "openbao-member-1", "baseharbor"); err == nil {
		t.Fatal("foreign external volume accepted")
	}
}

func TestResolveOwnedSpiloPGROOTVolume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compose.yaml")
	raw := "services:\n  postgres-member-1:\n    volumes:\n      - pg-data:/home/postgres/pgdata/pgroot\nvolumes:\n  pg-data: {}\n"
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	volume, err := ResolveOwnedServiceVolume(path, "postgres-member-1", "owned")
	if err != nil || volume != "owned_pg-data" {
		t.Fatalf("Spilo PGROOT: %q %v", volume, err)
	}
}
