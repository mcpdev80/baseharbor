package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
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
	err = appListCommand().Run(testOperatorContext(context.Background()), []string{"--all-targets"}, &out, &errOut)
	if err != nil {
		t.Fatalf("all-targets inventory depended on deleted Target config %q: %v", target.Name, err)
	}
	if out.String() != "No deployments registered.\n" {
		t.Fatalf("empty all-targets inventory = %q, want empty-install message", out.String())
	}
}

func TestAppListAllTargetsStillRequiresAuthorizationWithoutTargetConfig(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	configureTestTarget(t)
	configPath, err := deployment.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(configPath); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	err = appListCommand().Run(context.Background(), []string{"--all-targets"}, &out, &errOut)
	if err == nil {
		t.Fatal("all-targets inventory succeeded without operator authorization after Target config removal")
	}
	// The missing-config inventory must NOT reach the operator: it has to be
	// denied at the operator-auth gate, not let through by tolerating
	// ErrConfigMissing before the operator decides. A swallowed
	// ErrConfigMissing here is the exact regression under fix.
	if errors.Is(err, deployment.ErrConfigMissing) {
		t.Fatalf("anonymous all-targets call must fail at the operator-auth gate, not surface ErrConfigMissing: %v", err)
	}
	if !strings.Contains(err.Error(), "Managed-environment machine operations require an authenticated BaseHarbor operator") {
		t.Fatalf("anonymous all-targets call must surface the operator-auth gate text; got %q", err.Error())
	}
	if out.Len() != 0 {
		t.Fatalf("unauthorized all-targets inventory wrote output: %q", out.String())
	}
}
