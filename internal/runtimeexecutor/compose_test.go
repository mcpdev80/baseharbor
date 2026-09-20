package runtimeexecutor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/openbao"
)

func TestComposeYAMLUsesNonRootPreparedStateVolume(t *testing.T) {
	got := composeYAML(
		"baseharbor-runtime:test",
		openbao.RuntimeExecutorMTLSFiles{
			CA:   "/tmp/ca.pem",
			Cert: "/tmp/executor-cert.pem",
			Key:  "/tmp/executor-key.pem",
		},
		"/tmp/s3-admin.env",
	)

	if strings.Contains(got, "state-init:") {
		t.Fatalf("runtime executor compose unexpectedly contains privileged state init service:\n%s", got)
	}
	for _, want := range []string{
		"runtime-resource-state:/var/lib/baseharbor/runtime-resources",
		"user: \"65532:65532\"",
		"read_only: true",
		"cap_drop:",
		"- ALL",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("runtime executor compose missing %q:\n%s", want, got)
		}
	}
}

func TestProjectContainerReadableSecretPreservesProtectedSource(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "executor")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "admin.env")
	if err := os.WriteFile(source, []byte("ACCESS_KEY_ID=a\nSECRET_ACCESS_KEY=b\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	projected, err := projectContainerReadableSecret(dir, source, "s3-admin.env", "S3 admin credentials")
	if err != nil {
		t.Fatal(err)
	}

	sourceInfo, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if got := sourceInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("source mode = %o, want 600", got)
	}
	projectedInfo, err := os.Stat(projected)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectedInfo.Mode().Perm(); got != 0o644 {
		t.Fatalf("projection mode = %o, want 644 inside protected directory", got)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("projection directory mode = %o, want 700", got)
	}
}
