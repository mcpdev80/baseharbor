package runtimebroker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/openbao"
)

func TestEnsureImageCreatesDefaultState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", "")

	got, err := ensureImage(dir)
	if err != nil {
		t.Fatalf("ensureImage() error = %v", err)
	}
	if got != DefaultImage {
		t.Fatalf("ensureImage() = %q, want %q", got, DefaultImage)
	}

	assertBrokerImageState(t, dir, DefaultImage)
}

func TestEnsureImageKeepsExistingStateWithoutExplicitOverride(t *testing.T) {
	dir := t.TempDir()
	writeBrokerImageState(t, dir, "ghcr.io/mcpdev80/baseharbor-runtime:0.2.0")
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", "")

	got, err := ensureImage(dir)
	if err != nil {
		t.Fatalf("ensureImage() error = %v", err)
	}
	want := "ghcr.io/mcpdev80/baseharbor-runtime:0.2.0"
	if got != want {
		t.Fatalf("ensureImage() = %q, want %q", got, want)
	}
	assertBrokerImageState(t, dir, want)
}

func TestEnsureImageKeepsExistingStateForSameExplicitOverride(t *testing.T) {
	dir := t.TempDir()
	want := "ghcr.io/mcpdev80/baseharbor-runtime:edge"
	writeBrokerImageState(t, dir, want)
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", want)

	got, err := ensureImage(dir)
	if err != nil {
		t.Fatalf("ensureImage() error = %v", err)
	}
	if got != want {
		t.Fatalf("ensureImage() = %q, want %q", got, want)
	}
	assertBrokerImageState(t, dir, want)
}

func TestEnsureImageReconcilesExplicitOverride(t *testing.T) {
	dir := t.TempDir()
	writeBrokerImageState(t, dir, "ghcr.io/mcpdev80/baseharbor-runtime:dev-local")
	want := "ghcr.io/mcpdev80/baseharbor-runtime:edge"
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", want)

	got, err := ensureImage(dir)
	if err != nil {
		t.Fatalf("ensureImage() error = %v", err)
	}
	if got != want {
		t.Fatalf("ensureImage() = %q, want %q", got, want)
	}
	assertBrokerImageState(t, dir, want)
}

func TestEnsureImageRejectsInvalidExplicitOverride(t *testing.T) {
	dir := t.TempDir()
	writeBrokerImageState(t, dir, "ghcr.io/mcpdev80/baseharbor-runtime:0.2.0")
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", "ghcr.io/mcpdev80/baseharbor-runtime:edge\nbad")

	_, err := ensureImage(dir)
	if err == nil || !strings.Contains(err.Error(), "BASEHARBOR_RUNTIME_IMAGE is invalid") {
		t.Fatalf("ensureImage() error = %v, want invalid override error", err)
	}

	assertBrokerImageState(t, dir, "ghcr.io/mcpdev80/baseharbor-runtime:0.2.0")
}

func TestEnsureImageRejectsInvalidPersistedState(t *testing.T) {
	dir := t.TempDir()
	writeBrokerImageState(t, dir, "")
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", "ghcr.io/mcpdev80/baseharbor-runtime:edge")

	_, err := ensureImage(dir)
	if err == nil || !strings.Contains(err.Error(), "runtime broker image state is invalid") {
		t.Fatalf("ensureImage() error = %v, want invalid state error", err)
	}
}

func writeBrokerImageState(t *testing.T, dir, image string) {
	t.Helper()
	path := filepath.Join(dir, "broker-image")
	if err := os.WriteFile(path, []byte(image+"\n"), 0o600); err != nil {
		t.Fatalf("write broker image state: %v", err)
	}
}

func assertBrokerImageState(t *testing.T, dir, want string) {
	t.Helper()
	path := filepath.Join(dir, "broker-image")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read broker image state: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != want {
		t.Fatalf("broker image state = %q, want %q", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat broker image state: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("broker image state mode = %o, want 600", got)
	}
}

func TestRuntimeDocsPolicy(t *testing.T) {
	t.Setenv("BASEHARBOR_RUNTIME_DOCS_ENABLED", "")
	for _, tc := range []struct {
		environment string
		want        bool
	}{
		{environment: "dev", want: true},
		{environment: "development", want: true},
		{environment: "test", want: false},
		{environment: "staging", want: false},
		{environment: "prod", want: false},
		{environment: "production", want: false},
	} {
		got, err := runtimeDocsEnabled(tc.environment)
		if err != nil {
			t.Fatalf("runtimeDocsEnabled(%q): %v", tc.environment, err)
		}
		if got != tc.want {
			t.Fatalf("runtimeDocsEnabled(%q) = %v, want %v", tc.environment, got, tc.want)
		}
	}

	t.Setenv("BASEHARBOR_RUNTIME_DOCS_ENABLED", "true")
	if got, err := runtimeDocsEnabled("prod"); err != nil || !got {
		t.Fatalf("explicit production opt-in = %v, %v", got, err)
	}
	t.Setenv("BASEHARBOR_RUNTIME_DOCS_ENABLED", "false")
	if got, err := runtimeDocsEnabled("dev"); err != nil || got {
		t.Fatalf("explicit development opt-out = %v, %v", got, err)
	}
	t.Setenv("BASEHARBOR_RUNTIME_DOCS_ENABLED", "not-bool")
	if _, err := runtimeDocsEnabled("dev"); err == nil {
		t.Fatal("invalid docs override should fail closed")
	}
}

func TestEnsureDocsPortIsStableAndOwnerOnly(t *testing.T) {
	dir := t.TempDir()
	files := application.RuntimeFiles{Dir: dir}
	m := application.New("demo", "dev", false, false, true)
	t.Setenv("BASEHARBOR_RUNTIME_DOCS_ENABLED", "")

	first, err := ensureDocsPort(m, files)
	if err != nil {
		t.Fatal(err)
	}
	if first == "" {
		t.Fatal("development docs port was not allocated")
	}
	second, err := ensureDocsPort(m, files)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("docs port changed: %q -> %q", first, second)
	}
	info, err := os.Stat(filepath.Join(dir, "broker-docs-port"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("docs port state mode = %o, want 600", got)
	}
	if got := docsURL(first); !strings.HasPrefix(got, "https://127.0.0.1:") {
		t.Fatalf("docs URL is not loopback-only: %q", got)
	}
}

func TestEnsureDocsPortDisabledOutsideDevelopment(t *testing.T) {
	dir := t.TempDir()
	files := application.RuntimeFiles{Dir: dir}
	m := application.New("demo", "prod", false, false, true)
	t.Setenv("BASEHARBOR_RUNTIME_DOCS_ENABLED", "")

	port, err := ensureDocsPort(m, files)
	if err != nil {
		t.Fatal(err)
	}
	if port != "" {
		t.Fatalf("production docs port = %q, want disabled", port)
	}
	if _, err := os.Stat(filepath.Join(dir, "broker-docs-port")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("production docs state should not exist: %v", err)
	}
}

func TestComposeYAMLUsesNonRootPreparedRuntimeOperationVolume(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	m := application.New("demo", "dev", false, false, false)
	m = application.WithRuntimePermission(m, "object-storage.s3/v1", []string{"api"}, "runtime.create", "runtime.get", "runtime.delete")
	mtls := openbao.RuntimeMTLSFiles{
		CA:         write("ca.pem"),
		BrokerCert: write("broker-cert.pem"),
		BrokerKey:  write("broker-key.pem"),
		ClientCert: write("client-cert.pem"),
		ClientKey:  write("client-key.pem"),
	}
	got, err := composeYAML(
		m,
		mtls,
		write("runtime-token"),
		"",
		write("permissions.json"),
		write("service-tokens.json"),
		"baseharbor-runtime:test",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "state-init:") {
		t.Fatalf("runtime broker compose unexpectedly contains privileged state init service:\n%s", got)
	}
	if !strings.Contains(got, "runtime-operations:/var/lib/baseharbor/runtime-operations") {
		t.Fatalf("runtime broker compose missing persistent operations volume:\n%s", got)
	}
	if !strings.Contains(got, "user: \"65532:65532\"") {
		t.Fatalf("runtime broker compose missing explicit non-root identity:\n%s", got)
	}
}

func TestEnsureRuntimePermissionsFileIsReadOnlyContainerProjection(t *testing.T) {
	root := t.TempDir()
	files := application.RuntimeFiles{
		Dir:      filepath.Join(root, "runtime"),
		Bindings: filepath.Join(root, "runtime", "bindings"),
	}
	if err := os.MkdirAll(files.Bindings, 0o700); err != nil {
		t.Fatal(err)
	}
	m := application.New("demo", "dev", false, false, false)
	m = application.WithRuntimePermission(m, "object-storage.s3/v1", []string{"api"}, "runtime.create")

	path, err := ensureRuntimePermissionsFile(m, files)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("runtime permissions projection mode = %o, want 644", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("runtime permissions directory mode = %o, want 700", got)
	}
}

func TestComposeYAMLBrokerHealthcheckPinsTLSHostnameToLoopback(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	m := application.New("demo", "dev", false, false, true)
	mtls := openbao.RuntimeMTLSFiles{
		CA:         write("ca.pem"),
		BrokerCert: write("broker-cert.pem"),
		BrokerKey:  write("broker-key.pem"),
		ClientCert: write("client-cert.pem"),
		ClientKey:  write("client-key.pem"),
	}
	got, err := composeYAML(
		m,
		mtls,
		write("runtime-token"),
		write("openbao.env"),
		write("permissions.json"),
		write("service-tokens.json"),
		"baseharbor-runtime:test",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "--resolve") ||
		!strings.Contains(got, "baseharbor-runtime:8443:127.0.0.1") ||
		!strings.Contains(got, "https://baseharbor-runtime:8443/readyz") {
		t.Fatalf("broker healthcheck is not DNS-independent while preserving TLS hostname:\n%s", got)
	}
}


func TestProjectOwnerOnlyFilePreservesCanonicalPrivateKeyProtection(t *testing.T) {
	root := t.TempDir()
	files := application.RuntimeFiles{
		Dir:      filepath.Join(root, "runtime"),
		Bindings: filepath.Join(root, "runtime", "bindings"),
	}
	if err := os.MkdirAll(files.Bindings, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "canonical-key.pem")
	if err := os.WriteFile(source, []byte("private-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	projected, err := projectOwnerOnlyFile(files, source, "broker-key.pem", "runtime broker private key")
	if err != nil {
		t.Fatal(err)
	}

	sourceInfo, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if got := sourceInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("canonical key mode = %o, want 600", got)
	}
	projectedInfo, err := os.Stat(projected)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectedInfo.Mode().Perm(); got != 0o644 {
		t.Fatalf("projected key mode = %o, want 644", got)
	}
	parentInfo, err := os.Stat(filepath.Dir(projected))
	if err != nil {
		t.Fatal(err)
	}
	if got := parentInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("projection directory mode = %o, want 700", got)
	}
}
