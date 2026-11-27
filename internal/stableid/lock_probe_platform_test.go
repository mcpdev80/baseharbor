package stableid

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
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

// TestProbeEnvDropsInheritedPATH proves the security property that card 0428-222
// rc3 closes: an inherited PATH must NOT override the toolchain probe's pinned
// PATH, because Go's env-dedup is last-wins and probeEnv now builds the child
// env via envpin.PinnedEnv (drop inherited, append pins last).
func TestProbeEnvDropsInheritedPATH(t *testing.T) {
	prev, had := os.LookupEnv("PATH")
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("PATH", prev)
		} else {
			_ = os.Unsetenv("PATH")
		}
	})
	// Opposite inherited PATH that a last-wins append would otherwise win.
	if err := os.Setenv("PATH", "/inherited/bin:/usr/local/oldbin"); err != nil {
		t.Fatal(err)
	}

	got := probeEnv()
	count := 0
	var finalPATH string
	for _, kv := range got {
		if i := strings.IndexByte(kv, '='); i >= 0 && kv[:i] == "PATH" {
			count++
			finalPATH = kv[i+1:]
		}
	}
	if count != 1 {
		t.Fatalf("probeEnv produced %d PATH entries; want exactly 1 (pin wins, inherited dropped)\n%s", count, got)
	}
	if finalPATH == "/inherited/bin:/usr/local/oldbin" {
		t.Fatalf("probeEnv returned the inherited PATH instead of the pinned one\n%s", got)
	}
	if !strings.HasSuffix(finalPATH, "/usr/local/bin") {
		t.Fatalf("probeEnv PATH does not end at the anchored prefix /usr/local/bin\n%s", got)
	}
}

// TestToolchainProbeReportsGoPrerequisiteWhenGoAbsent proves the fail-closed
// contract card 0428-299 hunt33-go-runtime-prereq closes: a Go toolchain absent
// from PATH must not surface a raw "executable file not found in $PATH"; the
// caller must instead learn that the Go toolchain is the missing prerequisite.
// The build step seam is injected with the exact *exec.Error the OS returns when
// `go` cannot be located, so the detection maps it to errGoToolchainMissing.
func TestToolchainProbeReportsGoPrerequisiteWhenGoAbsent(t *testing.T) {
	orig := runToolchainBuildFunc
	t.Cleanup(func() { runToolchainBuildFunc = orig })
	runToolchainBuildFunc = func(build *exec.Cmd) error {
		return &exec.Error{Err: exec.ErrNotFound}
	}
	_, err := runToolchainProbe("/unused")
	if err == nil {
		t.Fatal("expected a fail-closed error when the Go toolchain is absent")
	}
	if !strings.Contains(err.Error(), "Go toolchain") {
		t.Fatalf("error does not document the Go toolchain prerequisite: %v", err)
	}
}

// TestToolchainProbePreservesNonMissingBuildFailure proves the detection only
// rewrites the not-found case: any other build failure keeps its original
// message so unrelated failures (a broken toolchain, a probe-source compile
// error) are not masked as a missing-toolchain prerequisite.
func TestToolchainProbePreservesNonMissingBuildFailure(t *testing.T) {
	orig := runToolchainBuildFunc
	t.Cleanup(func() { runToolchainBuildFunc = orig })
	boom := errors.New("compiler exploded")
	runToolchainBuildFunc = func(build *exec.Cmd) error { return boom }
	_, err := runToolchainProbe("/unused")
	if !errors.Is(err, boom) {
		t.Fatalf("expected the original build failure to pass through, got %v", err)
	}
}
