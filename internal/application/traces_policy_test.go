package application

import "testing"

func TestHasTraceSignalWithoutOTLPConfigIsFalse(t *testing.T) {
	m := New("no-telemetry", "dev", false, false, false)
	if HasTraceSignal(m) {
		t.Fatal("manifest without OTLP config unexpectedly reports traces")
	}
}

func TestHasTraceSignalDetectsTraces(t *testing.T) {
	m := WithOTLPTelemetry(New("with-traces", "dev", false, false, false), "metrics", "traces")
	if !HasTraceSignal(m) {
		t.Fatal("manifest with OTLP traces signal was not detected")
	}
}
