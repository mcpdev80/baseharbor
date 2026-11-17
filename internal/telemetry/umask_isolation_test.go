package telemetry

import (
	"os"
	"syscall"
	"testing"
)

// TestMain pins a 022 umask so that the numbered temp directories created by
// testing.T.TempDir() are always non-group-writable, regardless of the host
// umask. The ownership checks exercised by these tests (ValidateOTLPTLSProjection
// and the OTLP projection parent walk) require non-group-writable, self-owned
// parent directories. CI runners use umask 022, but shared dev hosts may use 002,
// which would leave t.TempDir()'s numbered subdir group-writable (0775) and fail
// the ownership assertions. This keeps the package host-umask-independent without
// touching production code.
func TestMain(m *testing.M) {
	if prev := syscall.Umask(0o022); prev != 0o022 {
		defer syscall.Umask(prev)
	}
	os.Exit(m.Run())
}
