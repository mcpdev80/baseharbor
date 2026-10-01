package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestPreflightRepositoryWorkloadFailsFastForHTTPSContractGap(t *testing.T) {
	root := t.TempDir()
	compose := `services:
  demo-app:
    build: .
    labels:
      io.baseharbor.workload.protocol: "https"
`
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{
		Manifest: application.Manifest{
			Version: application.CurrentVersion,
			Name: "demo",
			Environment: "dev",
			Workload: application.WorkloadConfig{Compose: "compose.yaml", Services: []string{"demo-app"}},
		},
		ManifestPath: filepath.Join(root, "baseharbor.yaml"),
		RepositoryRoot: root,
		FromRepository: true,
	}
	err := preflightRepositoryWorkload(resolved)
	if err == nil {
		t.Fatal("expected HTTPS workload without exposure.http to fail preflight")
	}
	if !strings.Contains(err.Error(), "exposure.http") {
		t.Fatalf("error does not identify missing exposure.http contract: %v", err)
	}
}
