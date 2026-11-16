package podman

import (
	"os"
	"syscall"
	"testing"
)

// TestMain pins a 022 umask so that the numbered temp directories created by
// testing.T.TempDir() are always non-group-writable, independent of the host
// umask. The quadlet-retirement ownership assertions in
// TestRetiredQuadletDefinitionCannotReappearAtBootAndKeepsSiblingData and
// TestQuadletRetirementRejectsForeignModifiedOrReplacedOwner require
// non-group-writable, self-owned parents. CI runners use umask 022, shared dev
// hosts may use 002 (leaving t.TempDir()'s numbered subdir group-writable at
// 0775), which fails those assertions. Pinning umask keeps the test
// host-independent without touching production code.
func TestMain(m *testing.M) {
	if prev := syscall.Umask(0o022); prev != 0o022 {
		defer syscall.Umask(prev)
	}
	os.Exit(m.Run())
}
