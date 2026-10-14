package application

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestOTLPAuthorityChangesSeparateRetainedManagedIdentityAndExternalAuthorization(t *testing.T) {
	dir := t.TempDir()
	f := RuntimeFiles{Dir: dir, Env: filepath.Join(dir, "runtime.env"), ApplicationEnv: filepath.Join(dir, "application.env"), Bindings: filepath.Join(dir, "bindings")}
	for _, p := range []string{f.Env, f.ApplicationEnv} {
		if err := os.WriteFile(p, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := telemetryManifest()
	if err := MaterializeOTLPTLSBindingMaterial(m, f, []byte("fixture-ca"), []byte("fixture-certificate"), []byte("fixture-private-key")); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(f.Bindings, "telemetry", "client-key.pem")
	originalKey, _ := os.ReadFile(keyPath)
	if err := MaterializeExternalOTLPBinding(m, f, "https://external.example.test", "Authorization=Bearer selected"); err != nil {
		t.Fatal(err)
	}
	bound, ok, err := ExistingRuntimeOTLPBinding(m, f)
	if err != nil || !ok || bound.CAFile != "" || bound.ClientCertFile != "" || bound.ClientKeyFile != "" {
		t.Fatal("managed private identity was handed to external authority")
	}
	if key, err := os.ReadFile(keyPath); err != nil || !bytes.Equal(key, originalKey) {
		t.Fatal("retained managed identity was destroyed by external selection")
	}
	if err := MaterializeOTLPBinding(m, f, capability.ProviderOTelCollector, "https://localhost:4318", "https://collector:4318"); err != nil {
		t.Fatal(err)
	}
	values, err := readRuntimeEnv(f.Env)
	if err != nil || values["OTLP_HEADERS"] != "" {
		t.Fatal("external authorization survived in managed runtime projection")
	}
	appValues, err := loadApplicationEnvValues(f.ApplicationEnv)
	if err != nil || appValues["OTEL_EXPORTER_OTLP_HEADERS"] != "" {
		t.Fatal("external authorization survived in managed host projection")
	}
}
