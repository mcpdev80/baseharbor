package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
)

func TestRepositoryUpRejectsAuthoredIdentityBeforeManagementInitialization(t *testing.T) {
	configureTestTarget(t)
	t.Chdir(t.TempDir())
	if err := os.WriteFile("baseharbor.yaml", []byte("version: 1\napp:\n  name: invalid-input\n  environment: dev\n  id: developer-authored\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("compose.yaml", []byte("services:\n  api:\n    image: example/api\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err := runWithIO(machineNoninteractiveContext(context.Background()), []string{"up", "--yes", "--skip-memory-preflight"}, &output, &output)
	if err == nil || !strings.Contains(err.Error(), "app.id") {
		t.Fatalf("authored identity did not fail before management bootstrap: %v", err)
	}
	target, err := effectiveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coreinstallation.Load(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid application initialized management owner: %v", err)
	}
	if _, err := os.Stat(filepath.Join(".baseharbor", "apps")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid application allocated a source identity: %v", err)
	}
}

func TestRepositoryUpCoreOptionsCannotInitializeApplicationSource(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("compose.yaml", []byte("services:\n  api:\n    image: example/api\n"), 0600); err != nil {
		t.Fatal(err)
	}
	handled, err := routeRepositoryRuntimeUp(context.Background(), strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, runtimeUpOptions{Yes: true, HA: true})
	if !handled || err == nil || !strings.Contains(err.Error(), "--control-plane-only") {
		t.Fatalf("management option silently changed application scope: %v", err)
	}
	if _, err := os.Stat("baseharbor.yaml"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected management option initialized authored source: %v", err)
	}
}
