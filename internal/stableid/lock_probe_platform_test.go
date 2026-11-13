package stableid

import (
	"errors"
	"runtime"
	"testing"
)

func TestLinuxAMD64LockProbeUsesEmbeddedHelperWithoutGoFallback(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("embedded probe is Linux/amd64-specific")
	}
	originalEmbedded, originalToolchain := runEmbeddedBinaryProbeFunc, runToolchainProbeFunc
	t.Cleanup(func() {
		runEmbeddedBinaryProbeFunc, runToolchainProbeFunc = originalEmbedded, originalToolchain
	})
	probeErr := errors.New("embedded probe failure")
	runEmbeddedBinaryProbeFunc = func(string) (bool, error) { return false, probeErr }
	runToolchainProbeFunc = func(string) (bool, error) {
		t.Fatal("Linux/amd64 unexpectedly required a Go toolchain fallback")
		return false, nil
	}
	if _, err := lockProbeRunDefault("/unused"); !errors.Is(err, probeErr) {
		t.Fatalf("embedded probe failure was hidden: %v", err)
	}
}
