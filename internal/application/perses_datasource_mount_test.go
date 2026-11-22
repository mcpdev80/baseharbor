package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	"go.yaml.in/yaml/v3"
)

// writePersesDatasourceArtifacts materializes a provisioned Prometheus datasource
// into the datasource projection root the context dir derives, so the datasource
// mount resolver reports present and the gateway render must splice a read-only
// mount for it.
func writePersesDatasourceArtifacts(t *testing.T, contextDir, resourceProject string) {
	t.Helper()
	_, projectionRoot := datasourceMountRoots(contextDir, resourceProject)
	if err := os.MkdirAll(filepath.Join(projectionRoot, "provisioning"), 0o755); err != nil {
		t.Fatalf("make datasource provisioning dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectionRoot, "provisioning", "datasource.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write datasource provisioning file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectionRoot, "config.yaml"), []byte("{\"provisioning\":{\"folders\":[\"/etc/perses/provisioning\"],\"interval\":\"300s\"}}\n"), 0o644); err != nil {
		t.Fatalf("write datasource config.yaml: %v", err)
	}
}

// TestPersesDatasourceMountResolvesAgainstComposeContextDir pins the contract of
// card 0428-083 F1: the Prometheus datasource mounts the gateway Perses service
// are resolved against the compose CONTEXT DIR (RuntimeFiles.Dir), so the mount
// source, resolved relative to files.Dir, lands on the datasource projection
// root the apply boundary materialized. A mount that resolved against the
// resource-project alone or a fixed path would NOT point at that projection
// root when resolved from files.Dir.
func TestPersesDatasourceMountResolvesAgainstComposeContextDir(t *testing.T) {
	m := persesRuntimeManifest(t, true)
	if !m.Services.Perses {
		t.Fatal("test setup: manifest must select Perses")
	}

	base, err := os.MkdirTemp("", "perses-datasource-base")
	if err != nil {
		t.Fatalf("temp base dir: %v", err)
	}
	defer os.RemoveAll(base)

	dirA := filepath.Join(base, "a", "runtime")
	dirB := filepath.Join(base, "deep", "nested", "b", "runtime")
	for _, dir := range []string{dirA, dirB} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("make context dir %q: %v", dir, err)
		}
	}

	filesA, err := ensureRuntimeForDir(t, m, dirA)
	if err != nil {
		t.Fatalf("ensure runtime files A: %v", err)
	}
	filesB, err := ensureRuntimeForDir(t, m, dirB)
	if err != nil {
		t.Fatalf("ensure runtime files B: %v", err)
	}

	writePersesDatasourceArtifacts(t, filesA.Dir, filesA.ResourceProject)
	writePersesDatasourceArtifacts(t, filesB.Dir, filesB.ResourceProject)

	renderedA, err := RuntimeComposeYAMLForProject(m, filesA)
	if err != nil {
		t.Fatalf("render A: %v", err)
	}
	renderedB, err := RuntimeComposeYAMLForProject(m, filesB)
	if err != nil {
		t.Fatalf("render B: %v", err)
	}

	// Both renders must splice the datasource config mount into the Perses
	// service volumes.
	srcA := persesDatasourceMountConfigSource(t, renderedA)
	srcB := persesDatasourceMountConfigSource(t, renderedB)
	if srcA == "" || srcB == "" {
		t.Fatalf("both renders must splice a datasource config mount: A=%q B=%q", srcA, srcB)
	}

	// The contract: the mount source, resolved relative to the compose context
	// dir (files.Dir), MUST land on the datasource projection root the apply
	// boundary materialized. This is the F1 derivation — the mount is
	// compose-context-relative, so a consumer derives it from files.Dir alone.
	_, projRootA := datasourceMountRoots(filesA.Dir, filesA.ResourceProject)
	wantA := filepath.Join(projRootA, "config.yaml")
	gotA, err := filepath.EvalSymlinks(filepath.Join(filesA.Dir, srcA))
	if err != nil {
		t.Fatalf("eval symlink mount source A %q: %v", gotA, err)
	}
	wantAEval, err := filepath.EvalSymlinks(wantA)
	if err != nil {
		t.Fatalf("eval wanted path A %q: %v", wantA, err)
	}
	if gotA != wantAEval {
		t.Fatalf("datasource config mount must resolve against the compose context dir: files.Dir=%q source=%q -> %q, want %q", filesA.Dir, srcA, gotA, wantAEval)
	}

	_, projRootB := datasourceMountRoots(filesB.Dir, filesB.ResourceProject)
	wantB := filepath.Join(projRootB, "config.yaml")
	gotB, err := filepath.EvalSymlinks(filepath.Join(filesB.Dir, srcB))
	if err != nil {
		t.Fatalf("eval symlink mount source B %q: %v", gotB, err)
	}
	wantBEval, err := filepath.EvalSymlinks(wantB)
	if err != nil {
		t.Fatalf("eval wanted path B %q: %v", wantB, err)
	}
	if gotB != wantBEval {
		t.Fatalf("datasource config mount must resolve against the compose context dir: files.Dir=%q source=%q -> %q, want %q", filesB.Dir, srcB, gotB, wantBEval)
	}
}

// ensureRuntimeForDir builds RuntimeFiles whose compose context dir (files.Dir)
// is the given directory.
func ensureRuntimeForDir(t *testing.T, m Manifest, dir string) (RuntimeFiles, error) {
	t.Helper()
	store := Store{Root: filepath.Join(dir, "..", "..", "apps"), Namespace: "perses-datasource-mount"}
	files, err := EnsureRuntime(context.Background(), serviceissuer.New(t), store, m)
	if err != nil {
		return RuntimeFiles{}, err
	}
	return files, nil
}

// persesDatasourceMountConfigSource extracts the compose-context-relative source
// of the read-only datasource config mount from a rendered Perses service block.
func persesDatasourceMountConfigSource(t *testing.T, rendered string) string {
	t.Helper()
	var compose struct {
		Services map[string]struct {
			Volumes []string `yaml:"volumes"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &compose); err != nil {
		t.Fatalf("decode rendered compose: %v", err)
	}
	for _, svc := range compose.Services {
		for _, v := range svc.Volumes {
			if strings.HasSuffix(v, ":"+PersesDatasourceContainerConfigPath+":ro") {
				parts := strings.SplitN(v, ":", 3)
				if len(parts) == 3 {
					return strings.TrimPrefix(parts[0], "./")
				}
			}
		}
	}
	return ""
}
