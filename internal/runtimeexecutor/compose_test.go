package runtimeexecutor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/openbao"
)

func writeNonEmpty(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureFilesRejectsInjectableRuntimeImage(t *testing.T) {
	root := t.TempDir()
	write := func(name string, contents string) string {
		t.Helper()
		p := filepath.Join(root, name)
		writeNonEmpty(t, p, contents)
		return p
	}
	identity := openbao.RuntimeExecutorMTLSFiles{
		CA:         write("ca.pem", "ca"),
		Cert:       write("executor-cert.pem", "cert"),
		Key:        write("executor-key.pem", "key"),
		ClientCert: write("observer-client-cert.pem", "cc"),
		ClientKey:  write("observer-client-key.pem", "ck"),
	}
	admin := write("s3-admin.env", "key=value")
	trust := write("s3-ca.pem", "trust")
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", "baseharbor-runtime:test\n  inject: \"x\"\n")

	if _, err := EnsureFiles(filepath.Join(root, "data"), identity, admin, "https://seaweedfs-access:8443", trust); err == nil {
		t.Fatalf("EnsureFiles: injecting a second key into the executor image was accepted, want error")
	}
}

func TestEnsureFilesAcceptsCanonicalRuntimeImage(t *testing.T) {
	root := t.TempDir()
	write := func(name string, contents string) string {
		t.Helper()
		p := filepath.Join(root, name)
		writeNonEmpty(t, p, contents)
		return p
	}
	identity := openbao.RuntimeExecutorMTLSFiles{
		CA:         write("ca.pem", "ca"),
		Cert:       write("executor-cert.pem", "cert"),
		Key:        write("executor-key.pem", "key"),
		ClientCert: write("observer-client-cert.pem", "cc"),
		ClientKey:  write("observer-client-key.pem", "ck"),
	}
	admin := write("s3-admin.env", "key=value")
	trust := write("s3-ca.pem", "trust")
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", "ghcr.io/mcpdev80/baseharbor-runtime:0.4.28")

	files, err := EnsureFiles(filepath.Join(root, "data"), identity, admin, "https://seaweedfs-access:8443", trust)
	if err != nil {
		t.Fatalf("EnsureFiles with canonical image returned error: %v", err)
	}
	if files.Image != "ghcr.io/mcpdev80/baseharbor-runtime:0.4.28" {
		t.Fatalf("image = %q, want canonical", files.Image)
	}
}

func TestEnsureFilesRejectsEmptyProject3AdminCredentials(t *testing.T) {
	root := t.TempDir()
	write := func(name string, contents string) string {
		t.Helper()
		p := filepath.Join(root, name)
		writeNonEmpty(t, p, contents)
		return p
	}
	identity := openbao.RuntimeExecutorMTLSFiles{
		CA:         write("ca.pem", "ca"),
		Cert:       write("executor-cert.pem", "cert"),
		Key:        write("executor-key.pem", "key"),
		ClientCert: write("observer-client-cert.pem", "cc"),
		ClientKey:  write("observer-client-key.pem", "ck"),
	}
	// Empty source credential: the projected S3 admin file would be empty.
	admin := write("s3-admin.env", "")
	trust := write("s3-ca.pem", "trust")

	if _, err := EnsureFiles(filepath.Join(root, "data"), identity, admin, "https://seaweedfs-access:8443", trust); err == nil {
		t.Fatalf("EnsureFiles: empty projected S3 admin credentials accepted, want error")
	}
}

func TestComposeYAMLUsesNonRootPreparedStateVolume(t *testing.T) {
	got := composeYAML(
		"baseharbor-runtime:test",
		openbao.RuntimeExecutorMTLSFiles{
			CA:         "/tmp/ca.pem",
			Cert:       "/tmp/executor-cert.pem",
			Key:        "/tmp/executor-key.pem",
			ClientCert: "/tmp/observer-client-cert.pem",
			ClientKey:  "/tmp/observer-client-key.pem",
		},
		"/tmp/s3-admin.env",
		"https://seaweedfs-access:8443",
		"/tmp/s3-ca.pem",
	)

	if strings.Contains(got, "state-init:") {
		t.Fatalf("runtime executor compose unexpectedly contains privileged state init service:\n%s", got)
	}
	for _, want := range []string{
		"runtime-resource-state:/var/lib/baseharbor/runtime-resources",
		"read_only: true",
		"cap_drop:",
		"- ALL",
		"healthcheck:",
		"https://baseharbor-runtime-executor:9443/readyz",
		"--cert",
		"/run/baseharbor/observability/client-cert.pem",
		"--key",
		"/run/secrets/observer-client-key",
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

func TestProjectContainerReadablePublicFileAcceptsReadableTrustBundle(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "executor")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "ca.pem")
	if err := os.WriteFile(source, []byte("ca"), 0o644); err != nil {
		t.Fatal(err)
	}

	projected, err := projectContainerReadablePublicFile(dir, source, "s3-ca.pem", "S3 trust bundle")
	if err != nil {
		t.Fatal(err)
	}
	projectedInfo, err := os.Stat(projected)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectedInfo.Mode().Perm(); got != 0o644 {
		t.Fatalf("projection mode = %o, want 644", got)
	}
}

func TestEnsureFilesProjectsExecutorPrivateKeyForNonRootRuntime(t *testing.T) {
	root := t.TempDir()
	write := func(name string, mode os.FileMode) string {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(name), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		return path
	}
	identity := openbao.RuntimeExecutorMTLSFiles{
		CA:         write("ca.pem", 0o644),
		Cert:       write("executor-cert.pem", 0o644),
		Key:        write("executor-key.pem", 0o600),
		ClientCert: write("observer-client-cert.pem", 0o644),
		ClientKey:  write("observer-client-key.pem", 0o600),
	}
	admin := write("s3-admin.env", 0o600)
	trust := write("s3-ca.pem", 0o644)
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", "baseharbor-runtime:test")

	files, err := EnsureFiles(filepath.Join(root, "data"), identity, admin, "https://seaweedfs-access:8443", trust)
	if err != nil {
		t.Fatal(err)
	}

	projected := filepath.Join(files.Dir, "executor-key.pem")
	info, err := os.Stat(projected)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("projected executor key mode = %o, want 644", got)
	}
	sourceInfo, err := os.Stat(identity.Key)
	if err != nil {
		t.Fatal(err)
	}
	if got := sourceInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("canonical executor key mode = %o, want 600", got)
	}
	compose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(compose), projected) {
		t.Fatalf("runtime executor compose does not use projected private key:\n%s", compose)
	}
}

func TestComposeYAMLMountsObserverIdentityWithoutOTLP(t *testing.T) {
	got := composeYAML(
		"baseharbor-runtime:test",
		openbao.RuntimeExecutorMTLSFiles{
			CA:         "/tmp/ca.pem",
			Cert:       "/tmp/executor-cert.pem",
			Key:        "/tmp/executor-key.pem",
			ClientCert: "/tmp/observer-client-cert.pem",
			ClientKey:  "/tmp/observer-client-key.pem",
		},
		"/tmp/s3-admin.env",
		"https://seaweedfs-access:8443",
		"/tmp/s3-ca.pem",
	)

	for _, want := range []string{
		"/run/baseharbor/observability/client-cert.pem:ro",
		"observer-client-key",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("runtime executor compose missing observer mTLS binding %q without OTLP:\n%s", want, got)
		}
	}
	if strings.Contains(got, "OTEL_EXPORTER_OTLP_ENDPOINT") {
		t.Fatalf("runtime executor compose unexpectedly enables OTLP without binding:\n%s", got)
	}
}
