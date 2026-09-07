package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigReadsRuntimeEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.env")
	content := "BASEHARBOR_POSTGRES_DB=baseharbor\nBASEHARBOR_POSTGRES_USER=baseharbor\nBASEHARBOR_POSTGRES_PASSWORD=secret\nBASEHARBOR_POSTGRES_PORT=15432\nBASEHARBOR_OPENBAO_PORT=18200\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PostgresPort != 15432 || cfg.OpenBaoPort != 18200 {
		t.Fatalf("unexpected ports: %+v", cfg)
	}
	if cfg.PostgresPassword != "secret" {
		t.Fatal("postgres password not loaded")
	}
}

func TestLoadConfigRejectsInvalidPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.env")
	content := "BASEHARBOR_POSTGRES_DB=baseharbor\nBASEHARBOR_POSTGRES_USER=baseharbor\nBASEHARBOR_POSTGRES_PASSWORD=secret\nBASEHARBOR_POSTGRES_PORT=not-a-port\nBASEHARBOR_OPENBAO_PORT=18200\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected invalid port to fail")
	}
}
