package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNativeOTLPConsumerDiagnosticDoesNotPrintProtectedInputs(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("requires Python for the actual native consumer fixture")
	}
	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", filepath.Join(t.TempDir(), "private-trust-path"))
	t.Setenv("OTEL_EXPORTER_OTLP_CLIENT_KEY", "private-client-key-path")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "Authorization=Bearer private-fixture-token")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "https://private-endpoint.example")
	command := exec.Command(python, "-c", nativeOTLPConsumerPython, "00", "diagnostic-workload")
	output, err := command.CombinedOutput()
	if err == nil || !bytes.Contains(output, []byte(`"stage": "trust"`)) || !bytes.Contains(output, []byte(`"error_type": "FileNotFoundError"`)) {
		t.Fatalf("actual consumer lost its failure or safe stage: %s %v", output, err)
	}
	for _, protected := range []string{"private-trust-path", "private-client-key-path", "private-fixture-token", "private-endpoint.example", "Traceback"} {
		if bytes.Contains(output, []byte(protected)) {
			t.Fatal("native consumer diagnostic exposed protected input or raw traceback")
		}
	}
}
