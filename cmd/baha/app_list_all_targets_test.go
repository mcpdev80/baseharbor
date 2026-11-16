package main

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestAppListAllTargetsWorksAfterInstallationDestroy(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTarget(t)
	configPath, err := deployment.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(configPath); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	err = appListCommand().Run(context.Background(), []string{"--all-targets"}, &out, &errOut)
	if err != nil {
		t.Fatalf("all-targets inventory depended on deleted Target config %q: %v", target.Name, err)
	}
	if out.String() != "No deployments registered.\n" {
		t.Fatalf("empty all-targets inventory = %q, want empty-install message", out.String())
	}
}
