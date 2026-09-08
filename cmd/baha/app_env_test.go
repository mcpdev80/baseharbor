package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestAppEnvMasksSecretsByDefaultAndRevealsExplicitly(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	store := application.DefaultStore()
	m := application.New("demo", "dev", true, true, false)
	if _, err := store.Create(m); err != nil {
		t.Fatal(err)
	}
	files, err := application.EnsureRuntime(store, m)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "env", "demo"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "DATABASE_URL=<masked>") || !strings.Contains(text, "REDIS_URL=<masked>") {
		t.Fatalf("default output did not mask service credentials: %s", text)
	}
	if strings.Contains(text, "postgresql://") || strings.Contains(text, "redis://") {
		t.Fatalf("default output leaked service URLs: %s", text)
	}

	out.Reset()
	if err := runWithIO(context.Background(), []string{"app", "env", "demo", "--reveal", "--format", "json"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "postgresql://") || !strings.Contains(out.String(), "redis://") {
		t.Fatalf("explicit reveal did not return native service URLs: %s", out.String())
	}

	out.Reset()
	if err := runWithIO(context.Background(), []string{"app", "env", "demo", "--path"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != files.ApplicationEnv {
		t.Fatalf("unexpected application env path: %q want %q", strings.TrimSpace(out.String()), files.ApplicationEnv)
	}
	if !filepath.IsAbs(files.ApplicationEnv) {
		// DefaultStore is relative by design; consumers can still load this path from
		// the project working directory without any BaseHarbor runtime dependency.
		if !strings.HasSuffix(files.ApplicationEnv, filepath.Join("runtime", "application.env")) {
			t.Fatalf("unexpected relative application env path: %s", files.ApplicationEnv)
		}
	}
}

func TestParseAppEnvArgsRejectsUnsafeCombination(t *testing.T) {
	if _, _, _, _, err := parseAppEnvArgs([]string{"demo", "--path", "--reveal"}); err == nil {
		t.Fatal("expected --path with --reveal to fail")
	}
	if _, _, _, _, err := parseAppEnvArgs([]string{"demo", "--format", "toml"}); err == nil {
		t.Fatal("expected unsupported format to fail")
	}
}
