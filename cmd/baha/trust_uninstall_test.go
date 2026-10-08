package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestTrustUninstallRequiresExplicitConsent(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var out bytes.Buffer
	if err := trustUninstallCommand().Run(context.Background(), nil, &out, &out); err == nil {
		t.Fatal("uninstall without --yes must fail")
	}
	if strings.Contains(out.String(), "REMOVED") {
		t.Fatal("unconfirmed mutation")
	}
}

func TestTrustUninstallWithoutOwnedStateIsIdempotent(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	dir, err := bhruntime.DataDir("")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	for i := 0; i < 2; i++ {
		out.Reset()
		if err := trustUninstallCommand().Run(context.Background(), []string{"--yes"}, &out, &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "No BaseHarbor-owned") {
			t.Fatalf("unexpected output: %s", out.String())
		}
	}
}
