package logs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/providertopology"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestLokiHAWithoutStorageCannotWriteSingleTopology(t *testing.T) {
	root := filepath.Join(t.TempDir(), "uncreated")
	m := application.WithHA(application.New("topology", "dev", false, false, false), true)
	_, err := EnsureProviderFilesForModeAt(context.Background(), nil, root, "", m, bhruntime.LogCollectionSyslog)
	if err == nil || !strings.Contains(err.Error(), "object-storage binding") {
		t.Fatalf("HA without storage accepted: %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("rejected intent changed provider state")
	}
}

func TestLokiRetainedSingleRejectsHABeforeStoragePreparation(t *testing.T) {
	root := t.TempDir()
	m := application.WithHA(application.New("topology", "dev", false, false, false), true)
	p, err := PlacementForAt(root, "", m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(p.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := providerFiles(p).Compose
	single := "loki"
	if string(p.Scope) == "application" {
		single = "baseharbor-internal-loki"
	}
	before := []byte("services:\n  " + single + ":\n    image: grafana/loki:3\n")
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	r := &runtimeLokiRealization{app: m, dataDir: root}
	if _, err := r.ensureProviderFiles(context.Background()); err == nil || !strings.Contains(err.Error(), "topology") {
		t.Fatalf("retained single accepted: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatal("retained topology changed")
	}
	if _, err := providertopology.ServiceNames(path); err != nil {
		t.Fatal(err)
	}
}
